# Playground Ока — один образ, multi-stage.
# Контекст сборки — КОРЕНЬ репозитория (библиотека нужна внутри образа):
#
#   docker build -f playground/Dockerfile .
#
# Финальный слой — golang, не scratch: toolchain нужен в рантайме, чтобы
# компилировать код пользователя (~1 ГиБ — осознанная цена, см. SPEC §6).

# ── этап 1: фронтенд ─────────────────────────────────────────────────
FROM node:22-alpine AS frontend
WORKDIR /src
COPY playground/frontend/package.json playground/frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY playground/frontend/ ./
# outDir в vite.config смотрит в ../backend/web/dist — здесь бэкенда нет,
# собираем в локальный dist и передаём следующему этапу
RUN npm run build -- --outDir dist --emptyOutDir

# ── этап 2: сервер ───────────────────────────────────────────────────
FROM golang:1.24 AS backend
WORKDIR /src
COPY playground/backend/ ./
COPY --from=frontend /src/dist ./web/dist
RUN CGO_ENABLED=0 go build -o /pgserver .

# ── этап 3: рантайм ──────────────────────────────────────────────────
FROM golang:1.24
# библиотека Ока — цель replace в go.mod каждого снипетта
COPY go.mod LICENSE /eye-lib/
COPY eye.go explore.go options.go /eye-lib/
COPY internal/ /eye-lib/internal/
COPY --from=backend /pgserver /usr/local/bin/pgserver

# прогрев кеша сборки: компилируем эталонный снипетт при сборке образа,
# чтобы первый запуск пользователя не ждал компиляции всей stdlib
ENV GOCACHE=/eye-cache
RUN set -eu; \
    mkdir -p /tmp/warm && cd /tmp/warm; \
    printf 'module snippet\n\ngo 1.22\n\nrequire github.com/vitikevich-landau/go_magic_eye v0.0.0\n\nreplace github.com/vitikevich-landau/go_magic_eye => /eye-lib\n' > go.mod; \
    printf 'package main\n\nimport eye "github.com/vitikevich-landau/go_magic_eye"\n\nfunc main() { x := 42; eye.Inspect(&x) }\n' > main.go; \
    GOPROXY=off GOFLAGS=-mod=mod CGO_ENABLED=0 go build -o /dev/null .; \
    cd / && rm -rf /tmp/warm; \
    useradd --system --create-home eye; \
    chown -R eye:eye /eye-cache

USER eye
ENV EYE_PG_LIB=/eye-lib \
    EYE_PG_ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["pgserver"]
