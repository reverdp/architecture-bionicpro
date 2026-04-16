CREATE TABLE IF NOT EXISTS crm_users (
    username TEXT PRIMARY KEY,
    email TEXT NOT NULL,
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    prosthesis_id TEXT NOT NULL UNIQUE,
    prosthesis_type TEXT NOT NULL,
    market TEXT NOT NULL
);

INSERT INTO crm_users (
    username,
    email,
    first_name,
    last_name,
    prosthesis_id,
    prosthesis_type,
    market
)
VALUES
    ('prothetic1', 'prothetic1@example.com', 'Prothetic', 'One', 'BP-RU-001', 'hand', 'RU'),
    ('prothetic2', 'prothetic2@example.com', 'Prothetic', 'Two', 'BP-RU-002', 'hand', 'RU'),
    ('prothetic3', 'prothetic3@example.com', 'Prothetic', 'Three', 'BP-RU-003', 'hand', 'RU')
ON CONFLICT (username) DO NOTHING;
