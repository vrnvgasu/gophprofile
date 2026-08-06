# GophProfile — сервис аватарок

Микросервис для загрузки, хранения и раздачи аватарок пользователей.
Оригиналы лежат в S3-совместимом хранилище, метаданные — в PostgreSQL,
миниатюры создаются асинхронно воркером через Kafka.

## Стек

| Компонент | Решение |
|---|---|
| HTTP-роутинг | chi/v5 |
| База данных | PostgreSQL + pgx (через `database/sql`) |
| Миграции | goose, встроены в бинарник |
| Хранилище файлов | MinIO / S3 (minio-go) |
| Брокер сообщений | Kafka в режиме KRaft (franz-go) |
| Обработка изображений | disintegration/imaging |
| Логи | zap |
| Тесты | testify + go.uber.org/mock + go-sqlmock |

## Быстрый старт

Все окружение в контейнерах:

```sh
make up      # postgres + minio + kafka + server + worker
make down
```

Сервис поднимется на `http://localhost:8080`, веб-интерфейс — там же.
Консоль MinIO — `http://localhost:9001` (minioadmin / minioadmin).

Локальная разработка (инфраструктура в Docker, приложение — на хосте):

```sh
make infra-up
make run           # HTTP-сервер
make run-worker    # воркер, в отдельном терминале
```

Миграции применяются автоматически при старте сервера и воркера.

## API

| Метод | Путь | Описание |
|---|---|---|
| POST | `/api/v1/avatars` | загрузка аватарки, `multipart/form-data`, поле `file`, заголовок `X-User-ID` |
| GET | `/api/v1/avatars/{avatar_id}` | получение изображения, `?size=100x100\|300x300\|original` |
| GET | `/api/v1/avatars/{avatar_id}/metadata` | метаданные аватарки |
| DELETE | `/api/v1/avatars/{avatar_id}` | удаление, требует `X-User-ID` владельца |
| GET | `/api/v1/users/{user_id}/avatar` | последняя загруженная аватарка пользователя |
| DELETE | `/api/v1/users/{user_id}/avatar` | удаление последней аватарки пользователя |
| GET | `/api/v1/users/{user_id}/avatars` | список аватарок пользователя |
| GET | `/health` | состояние сервиса, БД, S3 и брокера |
| GET | `/web/upload` | страница с формой загрузки |
| POST | `/web/upload` | отправка формы: поля `userId` и `file`, редирект в галерею |
| GET | `/web/gallery/{user_id}` | галерея аватарок пользователя |
| GET | `/*` | веб-интерфейс из `web/static` |


Пример:

```sh
curl -X POST localhost:8080/api/v1/avatars -H "X-User-ID: user-1" -F "file=@photo.png"
curl "localhost:8080/api/v1/avatars/<id>?size=300x300" -o thumb.jpg
curl -X DELETE localhost:8080/api/v1/avatars/<id> -H "X-User-ID: user-1"
```

## Конфигурация

Переменные окружения (приоритетнее флагов CLI):

| Переменная | Флаг | По умолчанию |
|---|---|---|
| `RUN_ADDRESS` | `-a` | `:8080` |
| `DATABASE_URI` | `-d` | `postgres://gophprofile:gophprofile@localhost:5432/gophprofile?sslmode=disable` |
| `LOG_LEVEL` | `-l` | `info` |
| `STATIC_DIR` | `--static` | `web/static` |
| `MAX_UPLOAD_SIZE` | `--max-upload-size` | `10485760` |
| `S3_ENDPOINT` | `--s3-endpoint` | `localhost:9000` |
| `S3_ACCESS_KEY` | `--s3-access-key` | `minioadmin` |
| `S3_SECRET_KEY` | `--s3-secret-key` | `minioadmin` |
| `S3_BUCKET` | `--s3-bucket` | `avatars` |
| `S3_USE_SSL` | `--s3-use-ssl` | `false` |
| `KAFKA_BROKERS` | `--kafka-brokers` | `localhost:9092` |
| `KAFKA_TOPIC` | `--kafka-topic` | `avatar-events` |
| `KAFKA_GROUP_ID` | `--kafka-group` | `avatar-worker` |

## Разработка

```sh
make test          # тесты
make cover         # покрытие
make generate      # перегенерировать моки
make lint          # golangci-lint
make fmt           # golangci-lint fmt: gofmt + goimports
make vet           # go vet
make build         # бинарники в bin/
make install-lint  # поставить golangci-lint нужной версии
```


