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
| Логи | slog (JSON) + мост OpenTelemetry |
| Телеметрия | OpenTelemetry SDK → OTLP → OpenTelemetry Collector |
| Трейсинг | Jaeger |
| Метрики | Prometheus |
| Сбор логов | Loki |
| Визуализация | Grafana |
| Алерты на ошибки в логах | Loki ruler → Alertmanager |
| Тесты | testify + go.uber.org/mock + go-sqlmock |

## Архитектура

Сервер принимает загрузку, кладет оригинал в S3, метаданные — в PostgreSQL
и публикует событие в Kafka. Воркер читает событие, делает миниатюры 100x100
и 300x300 и дописывает их в метаданные. Оба сервиса шлют трейсы, метрики и логи
в OpenTelemetry Collector; наружу метрики отдает коллектор на порту 8889.

## Быстрый старт

Все окружение в контейнерах:

```sh
make up      # приложение + инфраструктура + стенд наблюдаемости
make down
```

Сервис поднимется на `http://localhost:8080`, веб-интерфейс — там же.

| Интерфейс | Адрес | Доступ |
|---|---|---|
| Веб-интерфейс и API | http://localhost:8080 | — |
| Grafana | http://localhost:3000 | admin / admin |
| Jaeger | http://localhost:16686 | — |
| Prometheus | http://localhost:9090 | — |
| Alertmanager | http://localhost:9093 | — |
| Консоль MinIO | http://localhost:9001 | minioadmin / minioadmin |

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

Машиночитаемая спецификация — [`api/openapi.yaml`](api/openapi.yaml).

Пример:

```sh
curl -X POST localhost:8080/api/v1/avatars -H "X-User-ID: user-1" -F "file=@photo.png"
curl "localhost:8080/api/v1/avatars/<id>?size=300x300" -o thumb.jpg
curl -X DELETE localhost:8080/api/v1/avatars/<id> -H "X-User-ID: user-1"
```

## Развертывание в Kubernetes

Нужны `minikube` (или другой кластер), `kubectl` и `helm`.

```sh
make k8s-up          # кластер minikube + аддоны ingress и metrics-server
make monitoring-up   # kube-prometheus-stack: Prometheus Operator, Prometheus, Grafana
make deploy          # собрать образ, залить в кластер и поставить чарт
```

Ingress отвечает на хосте `avatars.local` — его нужно прописать в `/etc/hosts`:

```sh
echo "$(minikube ip) avatars.local" | sudo tee -a /etc/hosts
curl http://avatars.local/health
```

Веб-интерфейс — `http://avatars.local/web/upload`, удалить релиз — `make undeploy`.

С внешней инфраструктурой ставится по `values-prod.yaml`: postgres, kafka и minio из чарта выключены, секреты передаются флагами.

```sh
helm upgrade --install gophprofile deployments/helm/gophprofile \
  -f deployments/helm/gophprofile/values-prod.yaml \
  --set secrets.databaseURI=... --set secrets.s3AccessKey=... --set secrets.s3SecretKey=...
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
| `RATE_LIMIT_RPS` | `--rate-limit-rps` | `20` (`0` — без ограничения) |
| `RATE_LIMIT_BURST` | `--rate-limit-burst` | `40` |
| `S3_ENDPOINT` | `--s3-endpoint` | `localhost:9000` |
| `S3_ACCESS_KEY` | `--s3-access-key` | `minioadmin` |
| `S3_SECRET_KEY` | `--s3-secret-key` | `minioadmin` |
| `S3_BUCKET` | `--s3-bucket` | `avatars` |
| `S3_USE_SSL` | `--s3-use-ssl` | `false` |
| `KAFKA_BROKERS` | `--kafka-brokers` | `localhost:9092` |
| `KAFKA_TOPIC` | `--kafka-topic` | `avatar-events` |
| `KAFKA_GROUP_ID` | `--kafka-group` | `avatar-worker` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `--otlp-endpoint` | пусто — телеметрия выключена |
| `OTEL_TRACES_SAMPLE_RATE` | `--trace-sample-rate` | `1.0` |

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
