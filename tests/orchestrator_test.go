package tests

import (
	"context"
	"testing"

	"github.com/Andreyka-coder9192/calc_go/internal/application"
	"github.com/Andreyka-coder9192/calc_go/proto/calc"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func setupOrchestrator(t *testing.T) (*application.Orchestrator, func()) {
	db, err := sqlx.Connect("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	schema := `
	CREATE TABLE users (
	  id INTEGER PRIMARY KEY AUTOINCREMENT,
	  login TEXT UNIQUE NOT NULL,
	  password_hash TEXT NOT NULL
	);
	CREATE TABLE expressions (
	  id INTEGER PRIMARY KEY AUTOINCREMENT,
	  user_id INTEGER NOT NULL,
	  expr TEXT NOT NULL,
	  status TEXT NOT NULL,
	  result REAL
	);
	CREATE TABLE tasks (
	  id TEXT PRIMARY KEY,
	  expr_id INTEGER NOT NULL,
	  arg1 REAL,
	  arg2 REAL,
	  operation TEXT,
	  operation_time INTEGER,
	  in_progress BOOLEAN NOT NULL DEFAULT 0,
	  done BOOLEAN NOT NULL DEFAULT 0,
	  UNIQUE(expr_id, arg1, arg2, operation)
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	orch := &application.Orchestrator{Config: application.ConfigFromEnv(), DB: db}
	return orch, func() { db.Close() }
}

func TestGetTask_NoTask(t *testing.T) {
	orch, teardown := setupOrchestrator(t)
	defer teardown()

	_, err := orch.GetTask(context.Background(), &calc.Empty{})
	st, _ := status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", st.Code())
	}
}

func TestPostResult_FullFlow(t *testing.T) {
	orch, teardown := setupOrchestrator(t)
	defer teardown()

	// Вставляем выражение и задачу
	res := orch.DB.MustExec(
		`INSERT INTO expressions(user_id, expr, status) VALUES (1, '(1+2)', 'pending')`,
	)
	exprID, _ := res.LastInsertId()
	orch.DB.MustExec(
		`INSERT INTO tasks(id, expr_id, arg1, arg2, operation, operation_time)
		 VALUES ('task1', ?, 1, 2, '+', 1)`,
		exprID,
	)

	// Получаем задачу
	taskResp, err := orch.GetTask(context.Background(), &calc.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if taskResp.Id != "task1" {
		t.Errorf("expected task1, got %s", taskResp.Id)
	}

	// Отправляем результат
	_, err = orch.PostResult(context.Background(), &calc.ResultReq{Id: "task1", Result: 3})
	if err != nil {
		t.Fatal(err)
	}

	// Проверка обновления выражения
	var statusStr string
	var resultVal float64
	if err := orch.DB.Get(&statusStr, "SELECT status FROM expressions WHERE id=?", exprID); err != nil {
		t.Fatal(err)
	}
	if err := orch.DB.Get(&resultVal, "SELECT result FROM expressions WHERE id=?", exprID); err != nil {
		t.Fatal(err)
	}
	if statusStr != "done" || resultVal != 3 {
		t.Errorf("expected done/3, got %s/%f", statusStr, resultVal)
	}
}
