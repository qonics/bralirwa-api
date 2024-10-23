CREATE TABLE IF NOT EXISTS users(
   id SERIAL PRIMARY KEY,
   fname text,
   lname text,
   phone text UNIQUE,
   email text UNIQUE,
   department_id INT REFERENCES departments(id),
   email_verified BOOLEAN DEFAULT FALSE,
   phone_verified BOOLEAN DEFAULT FALSE,
   locale text,
   avatar_url text,
   can_add_codes BOOLEAN DEFAULT FALSE,
   can_trigger_draw BOOLEAN DEFAULT FALSE,
   can_add_user BOOLEAN DEFAULT FALSE,
   can_view_logs BOOLEAN DEFAULT FALSE,
   password text,
   status text,
   address text,
   operator INT REFERENCES users(id) null,
   created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
   updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
   deleted_at TIMESTAMP
);
-- status: DISABLED,OKAY OR FRAUD
-- DEFAULT VALUE FOR TEXT IS NA (NOT_AVAILABLE)
create index users_email on users(email);
create index users_operator on users(operator);
create index users_department on users(department_id);
create index users_phone on users(phone);