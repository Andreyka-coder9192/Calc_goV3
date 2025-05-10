# Распределённый вычислитель арифметических выражений

Система для параллельного вычисления сложных арифметических выражений с использованием оркестратора и агентов-вычислителей.

## Установка и настройка
## Примеры сценариев

### Вычисление сложного выражения
```bash
curl -X POST http://localhost:8080/api/v1/calculate \
  -H "Content-Type: application/json" \
  -d '{"expression": "((3+5)*2-8)/4"}'

# Через 1.5 секунды:
curl http://localhost:8080/api/v1/expressions/1
# Ответ: {"status":"completed","result":2}
```

### Обработка ошибки
```bash
curl -X POST http://localhost:8080/api/v1/calculate \
  -d '{"expression": "10/(5-5)"}'

# Результат:
{
    "id": "2",
    "status": "error",
    "result": null
}
```

## Тестирование
```bash
# Запуск интеграционных тестов
go test -v ./tests/...

# Запуск нагрузочного теста
wrk -t4 -c100 -d30s http://localhost:8080/api/v1/expressions
```
1. **Клонируйте репозиторий**:
    ```bash
    git clone https://github.com/Andreyka-coder9192/calc_goV3.git
    cd calc_go
    ```

2. **Убедитесь, что установлен Go 1.20+**:
    ```bash
    go version
    ```
    Если Go не установлен:
    - **Linux/macOS**: [Официальная инструкция](https://go.dev/doc/install)
    - **Windows**: [Скачайте установщик](https://go.dev/dl/)

3. **Установите Docker и Docker Compose (опционально для контейнеризации).**
---

## Архитектура

```mermaid
graph LR
    U[Пользователь] -->|POST /api/v1/calculate| O[Оркестратор (HTTP 8080)]
    U -->|GET /api/v1/expressions| O
    O -->|gRPC GetTask| A1[Агент 1]
    O -->|gRPC GetTask| A2[Агент 2]
    A1 -->|gRPC PostResult| O
    A2 -->|gRPC PostResult| O
    O -->|GET /api/v1/expressions/{id}| U
```

**Оркестратор** (порт 8080 по умолчанию):

- Принимает выражения через REST API
- Разбивает выражения на атомарные задачи
- Управляет очередью задач через gRPC
- Собирает результаты через gRPC
- Хранит статусы вычислений

**Агенты**:

- Подключаются к оркестратору через gRPC
- Получают задачи через `GetTask` (gRPC)
- Выполняют арифметические операции с задержкой
- Возвращают результаты через `PostResult` (gRPC)
- Поддерживают параллельное выполнение задач

## Требования

- Go 1.20+
- Поддерживаемые операции: `+`, `-`, `*`, `/`
- Приоритет операций и скобки
- Параллельное выполнение операций

# Запуск системы

## 1. Запуск оркестратора

### Linux / macOS (bash)
~~~bash
# Установка времени операций (в миллисекундах)
export TIME_ADDITION_MS=200
export TIME_SUBTRACTION_MS=200
export TIME_MULTIPLICATIONS_MS=300
export TIME_DIVISIONS_MS=400

# Запуск оркестратора
go run ./cmd/orchestrator/main.go
~~~

### Windows (cmd.exe)
~~~bat
:: Установка времени операций (в миллисекундах)
set TIME_ADDITION_MS=200
set TIME_SUBTRACTION_MS=200
set TIME_MULTIPLICATIONS_MS=300
set TIME_DIVISIONS_MS=400

:: Запуск оркестратора
go run .\cmd\orchestrator\main.go
~~~

### Windows (PowerShell)
~~~powershell
# Установка времени операций (в миллисекундах)
$env:TIME_ADDITION_MS = "200"
$env:TIME_SUBTRACTION_MS = "200"
$env:TIME_MULTIPLICATIONS_MS = "300"
$env:TIME_DIVISIONS_MS = "400"

# Запуск оркестратора
go run .\cmd\orchestrator\main.go
~~~

## 2. Запуск агента

### Linux / macOS (bash)
```bash
# Указание вычислительной мощности и адреса оркестратора
export COMPUTING_POWER=4
export ORCHESTRATOR_ADDR=localhost:50051  # gRPC порт

# Запуск агента
go run ./cmd/agent/main.go
```

### Windows (PowerShell)
```powershell
# Настройка параметров
$env:COMPUTING_POWER = "4"
$env:ORCHESTRATOR_ADDR = "localhost:50051"

# Запуск агента
go run .\cmd\agent\main.go
```


## Дополнительно: Запуск в Docker

Запуск через Docker Compose:
~~~bash
docker-compose up --build
~~~

## Запуск фронтенда

Фронтенд запускается вместе с остальными сервисами через `docker-compose`. Для старта выполните команду:  
~~~bash
docker-compose up --build
~~~
Затем откройте браузер и перейдите по адресу http://localhost:8081.

После ввода выражения и нажатия "Вычислить" запрос отправится на API (http://localhost:8080/api/v1/calculate).

## API Endpoints (REST)

### 1. Добавление выражения
```bash
POST /api/v1/calculate
Content-Type: application/json

{"expression": "(2+3)*4-10/2"}
```

Ответ:
```json
{
    "id": "1",
    "status": "pending"
}
```

### 2. Получение списка выражений
```bash
GET /api/v1/expressions
```

Ответ:
```json
{
    "expressions": [
        {
            "id": "1",
            "expression": "(2+3)*4-10/2",
            "status": "completed",
            "result": 15
        }
    ]
}
```

### 3. Получение статуса по ID
```bash
GET /api/v1/expressions/1
```

Ответ:
```json
{
    "id": "1",
    "status": "completed",
    "result": 15
}
```

## Внутреннее API (для агентов)

### 1. Получение задачи

```bash
GET /internal/task
```

Пример ответа (200):

```json
{
    "task": {
        "id": "5",
        "arg1": 2,
        "arg2": 3,
        "operation": "+",
        "operation_time": 200
    }
}
```

### 2. Отправка результата

```bash
POST /internal/task
```

Пример запроса:

```json
{
  "id": "5",
  "result": 5
}
```

## Переменные окружения

### Оркестратор

- `PORT` - порт сервера (по умолчанию 8080)
- `TIME_ADDITION_MS` - время сложения (мс)
- `TIME_SUBTRACTION_MS` - время вычитания (мс)
- `TIME_MULTIPLICATIONS_MS` - время умножения (мс)
- `TIME_DIVISIONS_MS` - время деления (мс)

### Агент

- `ORCHESTRATOR_URL` - URL оркестратора
- `COMPUTING_POWER` - количество параллельных задач

## Примеры сценариев

### Вычисление сложного выражения
```bash
curl -X POST http://localhost:8080/api/v1/calculate \
  -H "Content-Type: application/json" \
  -d '{"expression": "((3+5)*2-8)/4"}'

# Через 1.5 секунды:
curl http://localhost:8080/api/v1/expressions/1
# Ответ: {"status":"completed","result":2}
```

### Обработка ошибки
```bash
curl -X POST http://localhost:8080/api/v1/calculate \
  -d '{"expression": "10/(5-5)"}'

# Результат:
{
    "id": "2",
    "status": "error",
    "result": null
}
```

## Тестирование
```bash
# Запуск интеграционных тестов
go test -v ./tests/...
```
