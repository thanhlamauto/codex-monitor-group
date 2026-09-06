# Operations

Back up the `postgres_data` Docker volume and Caddy data. Audit rows have no delete route but database administrators remain trusted. Monitor `/healthz`, container health, disk, and PostgreSQL backups.

To upgrade, pull a reviewed release and run `docker compose up -d --build`. Existing database tables are preserved. For the MVP, review model changes before deployment because automatic schema migration is not yet included.
