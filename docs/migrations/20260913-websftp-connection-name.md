# WebSFTP named connections

Additive change to `webssh_credentials`: nullable `name VARCHAR(128) DEFAULT ''`.
The existing schema initializer applies this field for SQLite and MySQL. Legacy
rows retain their account names as display fallbacks. Existing encrypted fields
and the `(proxy_id, user_id, username)` unique key are unchanged. One connection
per account per user per access is supported; editing does not rename the account.

Manual expansion, if migrations are managed externally:

```sql
ALTER TABLE webssh_credentials ADD COLUMN name VARCHAR(128) DEFAULT '';
```

Back up the database before rollout. On application rollback retain the additive
column; do not drop credential data. A separately approved schema rollback after
all readers stop using names is `ALTER TABLE webssh_credentials DROP COLUMN name`.
Connections without stored passwords contain empty encrypted material, never
plaintext. Legacy terminal access remains unchanged; profile writes require WebSFTP.
