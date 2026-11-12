#!/bin/sh

set -e

echo "Waiting for PostgreSQL..."

until pg_isready -h postgres -U postgres
do
    sleep 2
done

echo "Running migrations..."

goose \
    -dir sql/schema \
    postgres \
    "$DB_URL" \
    up

echo "Migrations completed."