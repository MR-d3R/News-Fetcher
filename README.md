# Сервис-агрегатор новостей

Это приложение-микросервис на Go, которое извлекает, обрабатывает и сохраняет новостные статьи из различных API и RSS-каналов. Приложение использует распределённую архитектуру с RabbitMQ для организации очереди сообщений, PostgreSQL для постоянного хранения и Redis для кэширования.

## Архитектура

Приложение состоит из трех основных сервисов:

1. **Producer** - Обрабатывает запросы API и планирует задачи по извлечению контента
2. **Content Fetcher** - Берёт задачи и извлекает необработанный контент из новостных источников
3. **Content Processor** - Обрабатывает сырой контент, классифицирует статьи и сохраняет их в базе данных

## Функции

- Получение статей из источников News API
- Анализ и обработка RSS-лент
- Автоматическая категоризация контента (программирование, политика, криптовалюта)
- Постоянное хранилище в PostgreSQL
- Кэширование с помощью Redis
- Распределенная обработка задач с помощью RabbitMQ
- RESTful API для извлечения статей

## Требования

- Go 1.21+
- Docker и Docker Compose
- PostgreSQL
- Redis
- RabbitMQ

## Конфигурация

### Переменные среды

Создайте файл `.env` в корневом каталоге (пример уже есть) такого вида:

```bash
PG_USER=postgres
PG_PASSWORD=YourSecurePassword
PG_DB=newsdb
RABBITMQ_USER=guest
RABBITMQ_PASS=guest
```

### Конфигурационный файл

Приложение использует файл `config.yaml` для тонкой настройки:

```yaml
Server:
  addr: "0.0.0.0"
  port: ":8080"

Rabbit:
  addr: "amqp://guest:guest@rabbitmq:5672/"

Postgres:
  addr: "postgres://postgres:YourSecurePassword@postgres:5432/newsdb"
  user: "postgres"
  password: "YourSecurePassword"
  db_name: "newsdb"

Redis:
  addr: "redis:6379"
  user:
  password:
  db: 0
  maxRetries: 5
  dialTimeout: 10
  timeout: 5

Logger:
  level: "debug"

NewsAPIKey: "your_news_api_key"
```

## Начало работы

### Создание и запуск с помощью Docker

1. Клонируем репозиторий
2. Создаем и запускаем службы с помощью Docker Compose:

```bash
docker-compose up -d
```

После этого запустятся все необходимые службы, ничего дополнительно устанавливать не нужно:

- База данных PostgreSQL
- Кэш Redis
- Брокер сообщений RabbitMQ
- Служба создания новостного API
- Служба загрузки контента
- Служба обработки контента

3. Можно приступать к работе!

### Сборка и запуск вручную

1. Установите зависимости:

```bash
go mod download
```

2. Создайте сервисы:

```bash
go build -o bin/producer cmd/producer/main.go
go build -o bin/content_fetcher cmd/content_fetcher_cns/main.go
go build -o bin/content_processor cmd/content_process_cns/main.go
```

3. Запустите каждую службу:

```bash
./bin/producer
./bin/content_fetcher
./bin/content_processor
```

## Конечные точки API

### Проверка работоспособности

```bash
GET /health
```

Возвращает статус сервиса producer.

### Обработка статей по URL

```bash
POST /article
Content-Type: application/json

{
  "url": "https://newsapi.org/v2/top-headlines?country=us"
}
```

Назначает задачу для получения статей по указанному URL.

### Получение статьи по идентификатору

```bash
GET /article/:id
```

Извлекает определенную статью по ее идентификатору.

### Получение последних статей

```bash
GET /article/recent
```

Извлекает 50 самых последних статей.

## Архитектура проекта

1. Клиент отправляет запрос на получение статей из источника
2. Producer создает задачу выборки и отправляет ее в очередь "content_fetch"
3. Программа выборки контента использует задачу, извлекает содержимое и отправляет его в очередь "content_process"
4. Обработчик содержимого использует сырую информацию, анализирует её, классифицирует статьи и сохраняет их в PostgreSQL
5. Redis используется для кэширования для повышения производительности

## Расширения и улучшения

Возможные улучшения для проекта:

1. Добавить аутентификацию для конечных точек API
2. Внедрить ограничение скорости
3. Добавить мониторинг и показатели
4. Создать внешний интерфейс для просмотра статей
5. Добавить поддержку большего количества источников и форматов новостей
6. Реализовать полнотекстовый поиск
7. Реализовать систему тегов к статьям и более продвинутую категоризацию

## Устранение неполадок

### Распространенные проблемы

1. **Проблемы с подключением**: Убедитесь, что все службы (PostgreSQL, Redis, RabbitMQ) запущены и доступны
2. **Ошибки с ключом API**: Убедитесь, что ваш ключ News API действителен и правильно настроен
3. **Проблемы с очередями**: Проверьте консоль управления RabbitMQ, чтобы убедиться, что очереди создаются правильно

### Logs

Файлы журналов хранятся в каталоге "logs/":

- `News API Producer.log`
- `Fetch Content consumer.log`
- `Process Content consumer.log`

## Лицензия

Этот проект лицензирован по лицензии MIT - подробности смотрите в файле LICENSE.
