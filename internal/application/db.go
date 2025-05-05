package application

import (
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func NewDB(path string) (*sqlx.DB, error) {
	db, err := sqlx.Connect("sqlite3", path)
	if err != nil {
		return nil, err
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
      done BOOLEAN NOT NULL DEFAULT 0,
      FOREIGN KEY(expr_id) REFERENCES expressions(id)
    );`
	_, err = db.Exec(schema)
	return db, err
}
