FROM golang:1.25

WORKDIR /app

# Install Goose
RUN go install github.com/pressly/goose/v3/cmd/goose@v3.27.2

ENV PATH="/root/go/bin:${PATH}"

# PostgreSQL client (for pg_isready)
RUN apt-get update && apt-get install -y postgresql-client

COPY sql ./sql

COPY migration/entrypoint.sh .

RUN chmod +x entrypoint.sh

CMD ["./entrypoint.sh"]