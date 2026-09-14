### English

# TelegramDirect Bot

A small, self-hosted Telegram bot for managing anonymous and non-anonymous suggestions.

The bot is written in **Go** as a single monolithic application and uses **SQLite** for persistent storage. It is designed to run on a VPS with minimal resource usage and no external services.

## Features

* Anonymous and non-anonymous suggestions
* Users interact with the bot through private messages
* Suggestions are sent to a dedicated admin group for moderation
* Multiple administrators can jointly manage the submission queue
* Inline buttons for approving or rejecting suggestions
* Approved suggestions are automatically published to a specified Telegram channel
* Administrator actions are logged directly in the admin group
* SQLite database for storing suggestions and configuration data
* Simple configuration through environment variables
* Interactive VPS installation script
* Automatic systemd service setup
* Suitable for small channels and communities

## Architecture

```text
User
 │
 │ Private message
 ▼
Telegram Bot
 │
 ▼
SQLite
 │
 ▼
Admin Group
 │
 ├── Approve ──────► Telegram Channel
 └── Reject
```

The bot does not require a web server, external database, or complex infrastructure.

## Installation

The project includes an installation script for Debian-based VPS systems.

The installer asks for:

* Telegram Bot Token
* Admin group chat ID
* Target channel ID
* Telegram IDs of the administrators

It then configures the bot, initializes the SQLite database, creates a systemd service, and starts the bot.

When entering Telegram IDs, you can use **Telegram Desktop's Developer Mode** to view chat and channel IDs.

## Configuration

Configuration is stored in an `.env` file. Sensitive values such as the bot token are not stored directly in the source code.

Example:

```env
BOT_TOKEN=your_bot_token
ADMIN_CHAT_ID=-1001234567890
CHANNEL_ID=-1009876543210
ADMIN_IDS=123456789,987654321
```

## Requirements

* Go
* SQLite
* Telegram Bot Token
* A VPS or other server capable of running a Go binary

The bot is intended to be lightweight and can run comfortably on a small VPS.

## License

See the `LICENSE` file for license information.



### Русский

# TelegramDirect Bot

Небольшой селф-хостед Telegram-бот для управления анонимными и неанонимными предложками.

Бот написан на **Go** в виде одного монолитного приложения и использует **SQLite** для постоянного хранения данных. Он рассчитан на запуск на VPS и не требует сторонних сервисов или сложной инфраструктуры.

## Возможности

* Анонимные и неанонимные предложки
* Пользователи взаимодействуют с ботом через личные сообщения
* Предложки отправляются в отдельную группу администраторов на модерацию
* Несколько администраторов могут совместно управлять очередью
* Inline-кнопки для одобрения и отклонения предложек
* Одобренные предложки автоматически публикуются в указанном Telegram-канале
* Действия администраторов логируются непосредственно в админской группе
* SQLite для хранения предложек и данных бота
* Простая конфигурация через переменные окружения
* Интерактивный скрипт установки на VPS
* Автоматическое создание systemd-сервиса
* Подходит для небольших каналов и сообществ

## Архитектура

```text
Пользователь
     │
     │ Личные сообщения
     ▼
Telegram-бот
     │
     ▼
  SQLite
     │
     ▼
Админская группа
     │
     ├── Одобрить ──────► Telegram-канал
     └── Отклонить
```

Боту не требуются веб-сервер, внешняя база данных или сложная инфраструктура.

## Установка

В проект входит установочный скрипт для VPS на базе Debian и других совместимых систем.

Во время установки скрипт запрашивает:

* токен Telegram-бота;
* ID админской группы;
* ID целевого канала;
* Telegram ID администраторов.

После этого он автоматически настраивает бота, создаёт SQLite-базу данных, устанавливает systemd-сервис и запускает бота.

Для получения Telegram ID можно использовать **Telegram Desktop и режим разработчика**. В нём доступны ID чатов и каналов.

## Конфигурация

Конфигурация хранится в `.env`. Секретные данные, например токен бота, не хранятся непосредственно в исходном коде.

Пример:

```env
BOT_TOKEN=your_bot_token
ADMIN_CHAT_ID=-1001234567890
CHANNEL_ID=-1009876543210
ADMIN_IDS=123456789,987654321
```

## Требования

* Go
* SQLite
* Telegram Bot Token
* VPS или другой сервер, способный запускать Go-бинарники

Бот рассчитан на небольшое потребление ресурсов и без проблем может работать на недорогом VPS.

## Лицензия

Информация о лицензии находится в файле `LICENSE`.
