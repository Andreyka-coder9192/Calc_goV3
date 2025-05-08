package application

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/Andreyka-coder9192/calc_go/pkg/calculation"
	"github.com/Andreyka-coder9192/calc_go/proto/calc"
	"github.com/jmoiron/sqlx"
	"github.com/rs/cors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Config holds server configuration values
type Config struct {
	Addr                string
	TimeAddition        int
	TimeSubtraction     int
	TimeMultiplications int
	TimeDivisions       int
}

// ConfigFromEnv reads config from environment
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

// Orchestrator implements both HTTP and gRPC servers
type Orchestrator struct {
	calc.UnimplementedCalcServer
	Config *Config
	db     *sqlx.DB
	mu     sync.Mutex
	// AST in-memory store for scheduling
	exprCounter int64
	taskCounter int64
}

// NewOrchestrator initializes DB and returns orchestrator
func NewOrchestrator() *Orchestrator {
	db, err := sqlx.Connect("sqlite3", "calcgo.db")
	if err != nil {
		log.Fatal("cannot connect to db:", err)
	}
	// migrate
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
  done BOOLEAN NOT NULL DEFAULT 0,
  UNIQUE(expr_id, arg1, arg2, operation),
  FOREIGN KEY(expr_id) REFERENCES expressions(id)
);
`
	if _, err := db.Exec(schema); err != nil {
		log.Fatal("migrate failed:", err)
	}
	return &Orchestrator{Config: ConfigFromEnv(), db: db}
}

// RegisterHandler handles POST /api/v1/register
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
	if _, err := o.db.Exec("INSERT INTO users(login,password_hash) VALUES(?,?)", req.Login, hash); err != nil {
		http.Error(w, "user exists", http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// LoginHandler handles POST /api/v1/login
func (o *Orchestrator) LoginHandler(w http.ResponseWriter, r *http.Request) {
	var req struct{ Login, Password string }
	json.NewDecoder(r.Body).Decode(&req)
	var id int
	var hash string
	err := o.db.Get(&hash, "SELECT password_hash FROM users WHERE login=?", req.Login)
	if err != nil {
		http.Error(w, "invalid creds", http.StatusUnauthorized)
		return
	}
	err = CheckPassword(hash, req.Password)
	if err != nil {
		http.Error(w, "invalid creds", http.StatusUnauthorized)
		return
	}
	// get user id
	o.db.Get(&id, "SELECT id FROM users WHERE login=?", req.Login)
	tok, err := CreateToken(id)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": tok})
}

// CalculateHandler creates expression + tasks
func (o *Orchestrator) CalculateHandler(w http.ResponseWriter, r *http.Request) {
	uid := r.Context().Value("user_id").(int)
	var req struct{ Expression string }
	json.NewDecoder(r.Body).Decode(&req)
	ast, err := ParseAST(req.Expression)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	// insert expression
	res := o.db.MustExec("INSERT INTO expressions(user_id,expr,status) VALUES(?,?,?)", uid, req.Expression, "pending")
	exprID, _ := res.LastInsertId()
	// schedule tasks
	o.scheduleTasksDB(exprID, ast)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]int64{"id": exprID})
}

// schedulePendingTasksDB — находит в AST узлы, готовые к выполнению, и вставляет их в БД
func (o *Orchestrator) schedulePendingTasksDB(exprID int64, root *ASTNode) {
	// Рекурсивно обходим все узлы
	if root == nil || root.IsLeaf {
		return
	}
	// Сначала поймать дочерние
	o.schedulePendingTasksDB(exprID, root.Left)
	o.schedulePendingTasksDB(exprID, root.Right)

	// Если оба ребёнка листы И задача по этому узлу ещё не создана
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
		// Добавляем новую задачу
		o.db.Exec(
			`INSERT OR IGNORE INTO tasks
             (id, expr_id, arg1, arg2, operation, operation_time)
             VALUES (?, ?, ?, ?, ?, ?)`,
			n, exprID, root.Left.Value, root.Right.Value, root.Operator, opTime,
		)
		root.TaskScheduled = true
	}
}

// scheduleTasksDB walks AST, inserts tasks into DB when both children leaf
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
		o.db.MustExec(
			"INSERT INTO tasks(id,expr_id,arg1,arg2,operation,operation_time) VALUES(?,?,?,?,?,?)",
			n, exprID, node.Left.Value, node.Right.Value, node.Operator, opTime,
		)
		node.TaskScheduled = true
	}
}

// expressionsHandler GET /api/v1/expressions
func (o *Orchestrator) expressionsHandler(w http.ResponseWriter, r *http.Request) {
	uid := r.Context().Value("user_id").(int)
	var exprs []struct {
		ID     int      `db:"id" json:"id"`
		Expr   string   `db:"expr" json:"expression"`
		Status string   `db:"status" json:"status"`
		Result *float64 `db:"result" json:"result,omitempty"`
	}
	o.db.Select(&exprs, "SELECT id,expr,status,result FROM expressions WHERE user_id=?", uid)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"expressions": exprs})
}

// expressionByIDHandler GET /api/v1/expressions/{id}
func (o *Orchestrator) expressionByIDHandler(w http.ResponseWriter, r *http.Request) {
	uid := r.Context().Value("user_id").(int)
	id, _ := strconv.Atoi(r.URL.Path[len("/api/v1/expressions/"):])
	var expr struct {
		ID     int      `db:"id"`
		Status string   `db:"status"`
		Result *float64 `db:"result"`
	}
	err := o.db.Get(&expr, "SELECT id,status,result FROM expressions WHERE user_id=? AND id=?", uid, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"expression": expr})
}

// GetTask for gRPC
func (o *Orchestrator) GetTask(ctx context.Context, _ *calc.Empty) (*calc.TaskResp, error) {
	var t struct {
		ID            string `db:"id"`
		Arg1          float64
		Arg2          float64
		Operation     string
		OperationTime int `db:"operation_time"`
	}
	err := o.db.Get(&t, "SELECT id,arg1,arg2,operation,operation_time FROM tasks WHERE done=0 LIMIT 1")
	if err != nil {
		return nil, status.Error(codes.NotFound, "no task")
	}
	return &calc.TaskResp{Id: t.ID, Arg1: t.Arg1, Arg2: t.Arg2, Operation: t.Operation, OperationTime: int32(t.OperationTime)}, nil
}

// PostResult for gRPC
func (o *Orchestrator) PostResult(ctx context.Context, in *calc.ResultReq) (*calc.Empty, error) {
	// mark done
	o.db.Exec("UPDATE tasks SET done=1 WHERE id=?", in.Id)
	// TODO: update AST node and possibly expression result and new tasks
	return &calc.Empty{}, nil
}

// отдаём первую незавершённую задачу
func (o *Orchestrator) InternalGetTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "only GET allowed", http.StatusMethodNotAllowed)
		return
	}

	// Структура точно соответствует JSON, который ждёт агент
	var t struct {
		ID            string  `db:"id" json:"id"`
		Arg1          float64 `db:"arg1" json:"arg1"`
		Arg2          float64 `db:"arg2" json:"arg2"`
		Operation     string  `db:"operation" json:"operation"`
		OperationTime int     `db:"operation_time" json:"operation_time"`
	}

	// Забираем одну незавершённую задачу
	err := o.db.Get(&t, `
        SELECT id, arg1, arg2, operation, operation_time
          FROM tasks
         WHERE done = 0
         LIMIT 1
    `)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// Оборачиваем в ключ "task", как ждёт агент
	json.NewEncoder(w).Encode(map[string]interface{}{"task": t})
}

func (o *Orchestrator) InternalPostTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		ID     string  `json:"id"`
		Result float64 `json:"result"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}

	// 1. Узнаём exprID
	var exprID int64
	if err := o.db.Get(&exprID, "SELECT expr_id FROM tasks WHERE id = ?", payload.ID); err != nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	// 2. Обновляем done=1
	if _, err := o.db.Exec("UPDATE tasks SET done = 1 WHERE id = ?", payload.ID); err != nil {
		http.Error(w, "update task failed", http.StatusInternalServerError)
		return
	}

	// 3. Планируем только новые задачи для родительских узлов
	var fullExpr string
	_ = o.db.Get(&fullExpr, "SELECT expr FROM expressions WHERE id = ?", exprID)
	ast, _ := ParseAST(fullExpr)
	o.schedulePendingTasksDB(exprID, ast) // с UNIQUE на уровне БД дубликаты не создадутся

	// 4. Проверяем оставшиеся задачи
	var remaining int
	_ = o.db.Get(&remaining, "SELECT COUNT(*) FROM tasks WHERE expr_id = ? AND done = 0", exprID)

	// 5. Если больше нет — финальный расчёт
	if remaining == 0 {
		result, _ := calculation.Calc(fullExpr)
		o.db.Exec("UPDATE expressions SET status = ?, result = ? WHERE id = ?", "done", result, exprID)
	}

	w.WriteHeader(http.StatusOK)
}

// RunServer starts HTTP and gRPC
func (o *Orchestrator) RunServer() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/register", o.RegisterHandler)
	mux.HandleFunc("/api/v1/login", o.LoginHandler)
	mux.Handle("/api/v1/calculate", o.AuthMiddleware(http.HandlerFunc(o.CalculateHandler)))
	mux.Handle("/api/v1/expressions", o.AuthMiddleware(http.HandlerFunc(o.expressionsHandler)))
	mux.Handle("/api/v1/expressions/", o.AuthMiddleware(http.HandlerFunc(o.expressionByIDHandler)))

	mux.HandleFunc("/internal/task", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			o.InternalGetTask(w, r)
		case http.MethodPost:
			o.InternalPostTask(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	httpSrv := &http.Server{Addr: ":" + o.Config.Addr, Handler: cors.Default().Handler(mux)}
	go func() {
		log.Println("HTTP listening on", o.Config.Addr)
		if err := httpSrv.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()

	// gRPC
	lis, err := net.Listen("tcp", ":9090")
	if err != nil {
		return err
	}
	grpcSrv := grpc.NewServer()
	calc.RegisterCalcServer(grpcSrv, o)
	log.Println("gRPC listening on 9090")
	return grpcSrv.Serve(lis)
}
