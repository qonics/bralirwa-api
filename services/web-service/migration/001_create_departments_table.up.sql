CREATE TABLE departments(
   id SERIAL PRIMARY KEY,
   title text UNIQUE,
   created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
   updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
   deleted_at TIMESTAMP
);
create index departments_title on departments(title);