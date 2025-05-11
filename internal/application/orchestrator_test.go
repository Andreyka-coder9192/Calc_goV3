package application

import (
	"context"
	"testing"

	"github.com/Andreyka-coder9192/calc_go/proto/calc"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func setupOrchestrator(t *testing.T) (*Orchestrator, func()) {
	db, err := sqlx.Connect("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	// Миграция schema
	schema := `
    CREATE TABLE expressions (id INTEGER PRIMARY KEY, user_id INTEGER, expr TEXT, status TEXT, result REAL);
    CREATE TABLE tasks (id TEXT PRIMARY KEY, expr_id INTEGER, arg1 REAL, arg2 REAL, operation TEXT, operation_time INTEGER, done BOOLEAN);
    `
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	orch := &Orchestrator{Config: ConfigFromEnv(), db: db}
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

	// вставляем Expression и одну задачу
	res := orch.db.MustExec(`INSERT INTO expressions(user_id,expr,status) VALUES (1, '(1+2)', 'pending')`)
	exprID, _ := res.LastInsertId()
	orch.db.MustExec(`INSERT INTO tasks(id,expr_id,arg1,arg2,operation,operation_time) VALUES ('task1', ?, 1, 2, '+', 1)`, exprID)

	// Получаем таск по gRPC
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

	// Проверяем, что expression.status стало done и result == 3
	var statusStr string
	var resultVal float64
	orch.db.Get(&statusStr, "SELECT status FROM expressions WHERE id=?", exprID)
	orch.db.Get(&resultVal, "SELECT result FROM expressions WHERE id=?", exprID)
	if statusStr != "done" || resultVal != 3 {
		t.Errorf("expected done/3, got %s/%f", statusStr, resultVal)
	}
}
