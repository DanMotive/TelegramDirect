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
* SQLite database for storing suggestions and bot data
* Simple configuration through environment variables
* Interactive VPS installation script
* Choice between **systemd** and **PM2** for process management
* Automatic service configuration and startup
* Designed for small channels and communities

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

The bot does not require a web server, external database, or other complex infrastructure.

The bot uses **Telegram long polling**, so it does **not require an incoming HTTP port**. You do not need to open or reserve ports such as `8080`.

## Installation

The project includes an interactive installation script for **Debian-based VPS systems**.

The installer asks for:

* Telegram Bot Token
* Admin group chat ID
* Target channel ID
* Telegram IDs of the administrators
* Process manager: **systemd** or **PM2**

The installer then:

1. Installs the required dependencies
2. Builds the bot
3. Creates the required directories and SQLite database
4. Saves the configuration
5. Creates and configures the selected process manager
6. Starts the bot
7. Configures automatic startup after reboot

### Telegram IDs

When entering Telegram IDs, you can use **Telegram Desktop's Developer Mode** to identify chats and channels.

Enable it in:

**Settings → Advanced → Developer Mode**

The IDs of the admin group, target channel, and administrator accounts can then be obtained from Telegram.

## Configuration

Configuration is stored in an `.env` file.

Sensitive values such as the bot token are not stored directly in the source code.

Example:

```env
BOT_TOKEN=your_bot_token
ADMIN_CHAT_ID=-1001234567890
CHANNEL_ID=-1009876543210
ADMIN_IDS=123456789,987654321
```

## Process Management

TelegramDirect Bot supports two process managers.

### systemd

Recommended for a traditional Linux VPS installation.

```bash
systemctl status telegramdirect
journalctl -u telegramdirect -f
```

### PM2

PM2 can also be used to manage the bot process and automatically restart it when necessary.

```bash
pm2 status
pm2 logs telegramdirect
```

The installation script configures the selected option automatically.

## Requirements

* Debian-based Linux VPS
* Go
* SQLite
* Telegram Bot Token
* A Telegram admin group
* A Telegram channel where approved suggestions will be published

The bot is designed to be lightweight and can run comfortably on a small VPS.

## License

See the `LICENSE` file for license information.

---

# TelegramDirect Bot

Небольшой селф-хостед Telegram-бот для управления анонимными и неанонимными предложками.

Бот написан на **Go** в виде одного монолитного приложения и использует **SQLite** для постоянного хранения данных. Он рассчитан на запуск на VPS с минимальным потреблением ресурсов и не требует сторонних сервисов или сложной инфраструктуры.

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
* Выбор между **systemd** и **PM2** для управления процессом
* Автоматическая настройка и запуск выбранного менеджера
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

Боту не требуются веб-сервер, внешняя база данных или другая сложная инфраструктура.

Бот использует **long polling Telegram**, поэтому ему **не требуется входящий HTTP-порт**. Не нужно открывать или резервировать порт вроде `8080`.

## Установка

В проект входит интерактивный установочный скрипт для **Debian-based VPS**.

Во время установки скрипт запрашивает:

* токен Telegram-бота;
* ID админской группы;
* ID целевого канала;
* Telegram ID администраторов;
* способ управления процессом: **systemd** или **PM2**.

После этого установщик:

1. Устанавливает необходимые зависимости
2. Собирает бота
3. Создаёт необходимые директории и SQLite-базу
4. Сохраняет конфигурацию
5. Настраивает выбранный менеджер процессов
6. Запускает бота
7. Настраивает автоматический запуск после перезагрузки VPS

### Получение Telegram ID

Для получения Telegram ID можно использовать **Telegram Desktop и режим разработчика**.

Он находится в:

**Настройки → Продвинутые настройки → Режим разработчика**

С его помощью можно определить ID админской группы и целевого канала. В конфигурацию также добавляются Telegram ID администраторов.

## Конфигурация

Конфигурация хранится в `.env`.

Секретные данные, например токен бота, не хранятся непосредственно в исходном коде.

Пример:

```env
BOT_TOKEN=your_bot_token
ADMIN_CHAT_ID=-1001234567890
CHANNEL_ID=-1009876543210
ADMIN_IDS=123456789,987654321
```

## Управление процессом

TelegramDirect Bot поддерживает два варианта.

### systemd

Рекомендуемый вариант для обычного Linux VPS.

```bash
systemctl status telegramdirect
journalctl -u telegramdirect -f
```

### PM2

Также можно использовать PM2 для управления процессом и автоматического перезапуска бота при сбоях.

```bash
pm2 status
pm2 logs telegramdirect
```

Выбранный вариант автоматически настраивается установочным скриптом.

## Требования

* Debian-based Linux VPS
* Go
* SQLite
* Telegram Bot Token
* Telegram-группа для администраторов
* Telegram-канал для публикации одобренных предложек

Бот рассчитан на небольшое потребление ресурсов и может комфортно работать на недорогом VPS.

## Лицензия

Информация о лицензии находится в файле `LICENSE`.
