# CalcGo

CalcGo — распределённый вычислитель арифметических выражений с параллельной обработкой через оркестратора и агентов.

## Архитектура
```mermaid
graph TD
  Client[Клиент] -->|HTTP| Orchestrator[Оркестратор]
  Orchestrator -->|gRPC| Agent1[Агент 1]
  Orchestrator -->|gRPC| Agent2[Агент 2]
  Agent1 -->|gRPC| Orchestrator
  Agent2 -->|gRPC| Orchestrator
  Orchestrator -->|HTTP| Client
```

## Установка и запуск

1. **Клонируйте репозиторий**
    ```bash
    git clone https://github.com/Andreyka-coder9192/calc_go.git
    cd calc_go
    ```
2. **Требования**
    - Go 1.20+
    - Docker и Docker Compose (опционально)

3. **Запуск оркестратора**
    ```bash
    # Linux/macOS
    export TIME_ADDITION_MS=200
    export TIME_SUBTRACTION_MS=200
    export TIME_MULTIPLICATIONS_MS=300
    export TIME_DIVISIONS_MS=400
    go run ./cmd/orchestrator/main.go

    # Windows PowerShell
    $env:TIME_ADDITION_MS=200; $env:TIME_SUBTRACTION_MS=200; \
    $env:TIME_MULTIPLICATIONS_MS=300; $env:TIME_DIVISIONS_MS=400
    go run .\cmd\orchestrator\main.go
    ```

4. **Запуск агента**
    ```bash
    # Linux/macOS
    export COMPUTING_POWER=4
    export ORCHESTRATOR_URL=localhost:8080
    go run ./cmd/agent/main.go

    # Windows PowerShell
    $env:COMPUTING_POWER=4; $env:ORCHESTRATOR_URL=localhost:8080
    go run .\cmd\agent\main.go
    ```

5. **Запуск фронтенда**
    - Откройте `index.html` в любом браузере или запустите через статический сервер на порту 8081.

6. **Docker Compose (опционально)**
    ```bash
    docker-compose up --build
    ```

---

## API (REST)

### POST `/api/v1/calculate`
Запускает вычисление выражения.

- **Запрос**
    ```http
    POST /api/v1/calculate HTTP/1.1
    Content-Type: application/json
    Authorization: Bearer <token>

    {"expression":"(2+3)*4-10/2"}
    ```
- **Ответ** (HTTP 201)
    ```json
    {"id": 1}
    ```

### GET `/api/v1/expressions`
Возвращает все выражения пользователя.

- **Запрос**
    ```http
    GET /api/v1/expressions HTTP/1.1
    Authorization: Bearer <token>
    ```
- **Ответ** (HTTP 200)
    ```json
    {
      "expressions": [
        {"id":1, "expression":"(2+3)*4-10/2", "status":"done", "result":15}
      ]
    }
    ```

### GET `/api/v1/expressions/{id}`
Возвращает статус и результат по ID.

- **Запрос**
    ```http
    GET /api/v1/expressions/1 HTTP/1.1
    Authorization: Bearer <token>
    ```
- **Ответ** (HTTP 200)
    ```json
    {"expression": {"id":1, "status":"done", "result":15}}
    ```

---

## Примеры использования

### Простое выражение
```bash
curl -X POST http://localhost:8080/api/v1/calculate \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"expression":"((3+5)*2-8)/4"}'
# -> {"id":1}
curl http://localhost:8080/api/v1/expressions/1 -H "Authorization: Bearer $TOKEN"
# -> {"expression":{"id":1,"status":"done","result":2}}
```
### Ошибка деления на ноль
```bash
curl -X POST http://localhost:8080/api/v1/calculate \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"expression":"10/(5-5)"}'
# -> HTTP 422: invalid expression or result out of range
```

### Тестирование
```bash
go test -v ./cmd/agent
```

### Переменные окружения
| Переменная              | Описание                                  | По умолчанию     |
|-------------------------|-------------------------------------------|------------------|
| TIME_ADDITION_MS        | Задержка для операции "+" (мс)            | 100              |
| TIME_SUBTRACTION_MS     | Задержка для операции "-" (мс)            | 100              |
| TIME_MULTIPLICATIONS_MS | Задержка для операции "*" (мс)            | 100              |
| TIME_DIVISIONS_MS       | Задержка для операции "/" (мс)            | 100              |
| COMPUTING_POWER         | Число параллельных агентов                | 1                |
| ORCHESTRATOR_URL        | Адрес gRPC оркестратора (http://host:port)| localhost:8080   |