# Sprint 2 Smoke Test (Debian 13)

Ten runbook waliduje ścieżkę E2E dla Sprint 2:

- instalacja panelu (`aipanel install`),
- login admina,
- utworzenie strony (`/api/sites`),
- walidacja artefaktów Nginx/PHP-FPM/docroot,
- utworzenie bazy MariaDB (`/api/sites/{id}/databases`),
- usunięcie bazy i strony z pełnym cleanupem.

## 1) Build i deploy binarki

Uruchom lokalnie (na stacji developerskiej):

```bash
make build
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/aipanel-linux ./cmd/aipanel
scp bin/aipanel-linux robson@aipanel.onee.my:~/aipanel/aipanel
scp scripts/smoke-sprint2.sh robson@aipanel.onee.my:~/aipanel/smoke-sprint2.sh
```

## 2) Instalacja na serwerze Debian 13

Uruchom na serwerze:

```bash
cd ~/aipanel
chmod +x ./aipanel ./smoke-sprint2.sh
sudo ./aipanel install \
  --admin-email admin@example.com \
  --admin-password 'ChangeMe12345!'
```

Weryfikacja bazowa:

```bash
sudo systemctl status aipanel nginx mariadb php8.3-fpm --no-pager
curl -fsS http://127.0.0.1/health
```

## 3) Uruchomienie smoke testu Sprint 2

```bash
cd ~/aipanel
sudo ./smoke-sprint2.sh \
  --base-url http://127.0.0.1 \
  --admin-email admin@example.com \
  --admin-password 'ChangeMe12345!' \
  --php-version 8.3
```

Przykład z ręczną domeną i bez cleanupu (debug):

```bash
sudo ./smoke-sprint2.sh \
  --admin-email admin@example.com \
  --admin-password 'ChangeMe12345!' \
  --domain smoke-20260206.example.test \
  --php-version 8.4 \
  --keep
```

## 4) Oczekiwany wynik

Na końcu powinien pojawić się komunikat:

```text
==> Smoke test PASSED
```

Jeśli skrypt padnie, zostawi log kroku i odpowiedź API w stderr.

## 5) Szybki troubleshooting

```bash
sudo journalctl -u aipanel -u nginx -u mariadb -u php8.3-fpm -u php8.4-fpm -n 200 --no-pager
sudo nginx -t
sudo mariadb -e "SHOW DATABASES;"
```
