package application

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/Andreyka-coder9192/calc_go/proto/calc"
	"github.com/jmoiron/sqlx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Config struct {
	Addr                string
	TimeAddition        int
	TimeSubtraction     int
	TimeMultiplications int
	TimeDivisions       int
}

func ConfigFromEnv() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	ta, _ := strconv.Atoi(os.Getenv("TIME_ADDITION_MS"))
	if ta == 0 {
		ta = 100
	}
	ts, _ := strconv.Atoi(os.Getenv("TIME_SUBTRACTION_MS"))
	if ts == 0 {
		ts = 100
	}
	tm, _ := strconv.Atoi(os.Getenv("TIME_MULTIPLICATIONS_MS"))
	if tm == 0 {
		tm = 100
	}
	td, _ := strconv.Atoi(os.Getenv("TIME_DIVISIONS_MS"))
	if td == 0 {
		td = 100
	}
	return &Config{
		Addr:                port,
		TimeAddition:        ta,
		TimeSubtraction:     ts,
		TimeMultiplications: tm,
		TimeDivisions:       td,
	}
}

type Orchestrator struct {
	calc.UnimplementedCalcServer
	Config      *Config
	DB          *sqlx.DB
	mu          sync.Mutex
	exprCounter int64
	taskCounter int64
}

func NewOrchestrator() *Orchestrator {
	db, err := sqlx.Connect("sqlite3", "calcgo.db")
	if err != nil {
		log.Fatal("cannot connect to db:", err)
	}
	schema := `
CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	login TEXT UNIQUE NOT NULL,
	password_hash TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS expressions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	expr TEXT NOT NULL,
	status TEXT NOT NULL,
	result REAL,
	FOREIGN KEY(user_id) REFERENCES users(id)
);
CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  expr_id INTEGER NOT NULL,
  arg1 REAL,
  arg2 REAL,
  operation TEXT,
  operation_time INTEGER,
  in_progress BOOLEAN NOT NULL DEFAULT 0,
  done BOOLEAN NOT NULL DEFAULT 0,
  UNIQUE(expr_id, arg1, arg2, operation),
  FOREIGN KEY(expr_id) REFERENCES expressions(id)
);
`
	if _, err := db.Exec(schema); err != nil {
		log.Fatal("migrate failed:", err)
	}
	return &Orchestrator{Config: ConfigFromEnv(), DB: db}
}

func (o *Orchestrator) RegisterHandler(w http.ResponseWriter, r *http.Request) {
	var req struct{ Login, Password string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	hash, err := HashPassword(req.Password)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if _, err := o.DB.Exec("INSERT INTO users(login,password_hash) VALUES(?,?)", req.Login, hash); err != nil {
		http.Error(w, "user exists", http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (o *Orchestrator) LoginHandler(w http.ResponseWriter, r *http.Request) {
	var req struct{ Login, Password string }
	json.NewDecoder(r.Body).Decode(&req)
	var id int
	var hash string
	err := o.DB.Get(&hash, "SELECT password_hash FROM users WHERE login=?", req.Login)
	if err != nil {
		http.Error(w, "invalid creds", http.StatusUnauthorized)
		return
	}
	err = CheckPassword(hash, req.Password)
	if err != nil {
		http.Error(w, "invalid creds", http.StatusUnauthorized)
		return
	}
	o.DB.Get(&id, "SELECT id FROM users WHERE login=?", req.Login)
	tok, err := CreateToken(id)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": tok})
}

func (o *Orchestrator) CalculateHandler(w http.ResponseWriter, r *http.Request) {
	uid := r.Context().Value("user_id").(int)
	var req struct{ Expression string }
	json.NewDecoder(r.Body).Decode(&req)

	ast, err := ParseAST(req.Expression)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	result, err := EvalAST(ast)
	if err != nil || math.IsInf(result, 0) || math.IsNaN(result) {
		http.Error(w, "invalid expression or result out of range", http.StatusUnprocessableEntity)
		return
	}

	if ast.IsLeaf {
		res := o.DB.MustExec(
			"INSERT INTO expressions(user_id,expr,status,result) VALUES(?,?,?,?)",
			uid, req.Expression, "done", result,
		)
		id, _ := res.LastInsertId()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]int64{"id": id})
		return
	}

	res := o.DB.MustExec("INSERT INTO expressions(user_id,expr,status) VALUES(?,?,?)", uid, req.Expression, "pending")
	exprID, _ := res.LastInsertId()
	o.scheduleTasksDB(exprID, ast)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]int64{"id": exprID})
}

func (o *Orchestrator) schedulePendingTasksDB(exprID int64, root *ASTNode) {
	if root == nil || root.IsLeaf {
		return
	}
	o.schedulePendingTasksDB(exprID, root.Left)
	o.schedulePendingTasksDB(exprID, root.Right)

	if root.Left.IsLeaf && root.Right.IsLeaf && !root.TaskScheduled {
		n := strconv.FormatInt(time.Now().UnixNano(), 10)
		opTime := 0
		switch root.Operator {
		case "+":
			opTime = o.Config.TimeAddition
		case "-":
			opTime = o.Config.TimeSubtraction
		case "*":
			opTime = o.Config.TimeMultiplications
		case "/":
			opTime = o.Config.TimeDivisions
		}
		o.DB.Exec(
			`INSERT OR IGNORE INTO tasks
             (id, expr_id, arg1, arg2, operation, operation_time)
             VALUES (?, ?, ?, ?, ?, ?)`,
			n, exprID, root.Left.Value, root.Right.Value, root.Operator, opTime,
		)
		root.TaskScheduled = true
	}
}

func (o *Orchestrator) scheduleTasksDB(exprID int64, node *ASTNode) {
	if node == nil || node.IsLeaf {
		return
	}
	o.scheduleTasksDB(exprID, node.Left)
	o.scheduleTasksDB(exprID, node.Right)
	if node.Left.IsLeaf && node.Right.IsLeaf && !node.TaskScheduled {
		n := strconv.FormatInt(time.Now().UnixNano(), 10)
		var opTime int
		switch node.Operator {
		case "+":
			opTime = o.Config.TimeAddition
		case "-":
			opTime = o.Config.TimeSubtraction
		case "*":
			opTime = o.Config.TimeMultiplications
		case "/":
			opTime = o.Config.TimeDivisions
		}
		o.DB.MustExec(
			"INSERT INTO tasks(id,expr_id,arg1,arg2,operation,operation_time) VALUES(?,?,?,?,?,?)",
			n, exprID, node.Left.Value, node.Right.Value, node.Operator, opTime,
		)
		node.TaskScheduled = true
	}
}

func (o *Orchestrator) expressionsHandler(w http.ResponseWriter, r *http.Request) {
	uid := r.Context().Value("user_id").(int)
	var exprs []struct {
		ID     int      `db:"id" json:"id"`
		Expr   string   `db:"expr" json:"expression"`
		Status string   `db:"status" json:"status"`
		Result *float64 `db:"result" json:"result,omitempty"`
	}
	o.DB.Select(&exprs, "SELECT id,expr,status,result FROM expressions WHERE user_id=?", uid)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"expressions": exprs})
}

func (o *Orchestrator) expressionByIDHandler(w http.ResponseWriter, r *http.Request) {
	uid := r.Context().Value("user_id").(int)
	id, _ := strconv.Atoi(r.URL.Path[len("/api/v1/expressions/"):])
	var expr struct {
		ID     int      `db:"id"`
		Status string   `db:"status"`
		Result *float64 `db:"result"`
	}
	err := o.DB.Get(&expr, "SELECT id,status,result FROM expressions WHERE user_id=? AND id=?", uid, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"expression": expr})
}

func (o *Orchestrator) GetTask(ctx context.Context, _ *calc.Empty) (*calc.TaskResp, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	var t struct {
		ID            string  `db:"id"`
		Arg1          float64 `db:"arg1"`
		Arg2          float64 `db:"arg2"`
		Operation     string  `db:"operation"`
		OperationTime int     `db:"operation_time"`
	}
	err := o.DB.Get(&t, `
        SELECT id, arg1, arg2, operation, operation_time
          FROM tasks
         WHERE in_progress = 0 AND done = 0
         LIMIT 1
    `)
	if err != nil {
		return nil, status.Error(codes.NotFound, "no task")
	}
	if _, err := o.DB.Exec("UPDATE tasks SET in_progress = 1 WHERE id = ?", t.ID); err != nil {
		log.Printf("failed to mark task %s in progress: %v", t.ID, err)
	}

	return &calc.TaskResp{
		Id:            t.ID,
		Arg1:          t.Arg1,
		Arg2:          t.Arg2,
		Operation:     t.Operation,
		OperationTime: int32(t.OperationTime),
	}, nil
}

// PostResult — grpc-обработчик прихода результата от агента
func (o *Orchestrator) PostResult(ctx context.Context, in *calc.ResultReq) (*calc.Empty, error) {
	// 1. Узнаём, к какому выражению (expr_id) относится эта задача
	var exprID int64
	if err := o.DB.Get(&exprID, "SELECT expr_id FROM tasks WHERE id = ?", in.Id); err != nil {
		return nil, status.Error(codes.NotFound, "task not found")
	}

	// 2. Помечаем задачу как выполненную
	if _, err := o.DB.Exec("UPDATE tasks SET done = 1 WHERE id = ?", in.Id); err != nil {
		return nil, status.Error(codes.Internal, "failed to update task")
	}

	// 3. Перепланируем вновь доступные подзадачи из AST
	var fullExpr string
	if err := o.DB.Get(&fullExpr, "SELECT expr FROM expressions WHERE id = ?", exprID); err == nil {
		ast, err := ParseAST(fullExpr)
		if err == nil {
			o.schedulePendingTasksDB(exprID, ast)
		} else {
			log.Printf("PostResult: cannot parse AST: %v", err)
		}
	}

	// 4. Считаем, остались ли незавершённые задачи
	var remaining int
	if err := o.DB.Get(&remaining, "SELECT COUNT(*) FROM tasks WHERE expr_id = ? AND done = 0", exprID); err != nil {
		return nil, status.Error(codes.Internal, "failed to count tasks")
	}

	// 5. Если это была последняя — полностью вычисляем результат по AST
	if remaining == 0 {
		// парсим AST заново
		ast, err := ParseAST(fullExpr)
		if err != nil {
			log.Printf("PostResult: final AST parse error: %v", err)
		} else {
			result, err := EvalAST(ast)
			if err != nil {
				log.Printf("PostResult: AST evaluation error: %v", err)
			} else {
				// обновляем выражение в БД
				if _, err := o.DB.Exec(
					"UPDATE expressions SET status = ?, result = ? WHERE id = ?",
					"done", result, exprID,
				); err != nil {
					log.Printf("PostResult: failed to update expression: %v", err)
				}
			}
		}
	}

	return &calc.Empty{}, nil
}

func (o *Orchestrator) RunServer() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/register", o.RegisterHandler)
	mux.HandleFunc("/api/v1/login", o.LoginHandler)
	mux.Handle("/api/v1/calculate", o.AuthMiddleware(http.HandlerFunc(o.CalculateHandler)))
	mux.Handle("/api/v1/expressions", o.AuthMiddleware(http.HandlerFunc(o.expressionsHandler)))
	mux.Handle("/api/v1/expressions/", o.AuthMiddleware(http.HandlerFunc(o.expressionByIDHandler)))

	handlerWithCORS := EnableCORS(mux)

	httpSrv := &http.Server{
		Addr:    ":" + o.Config.Addr,
		Handler: handlerWithCORS,
	}
	go func() {
		log.Println("HTTP listening on", o.Config.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	lis, err := net.Listen("tcp", ":9090")
	if err != nil {
		return err
	}
	grpcSrv := grpc.NewServer()
	calc.RegisterCalcServer(grpcSrv, o)
	log.Println("gRPC listening on 9090")
	return grpcSrv.Serve(lis)
}

func EnableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
