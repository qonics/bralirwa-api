-- Insert provinces
INSERT INTO province (name) VALUES
('Northern Province'),
('Eastern Province'),
('Western Province'),
('Southern Province'),
('City of Kigali');

-- Insert districts with their province relationships
INSERT INTO district (name, province_id) VALUES
('Burera', 1),
('Gakenke', 1),
('Gicumbi', 1),
('Musanze', 1),
('Rulindo', 1),

('Bugesera', 2),
('Gatsibo', 2),
('Kayonza', 2),
('Kirehe', 2),
('Ngoma', 2),
('Nyagatare', 2),
('Rwamagana', 2),

('Karongi', 3),
('Ngororero', 3),
('Nyabihu', 3),
('Nyamasheke', 3),
('Rubavu', 3),
('Rutsiro', 3),
('Rusizi', 3),

('Gisagara', 4),
('Huye', 4),
('Kamonyi', 4),
('Muhanga', 4),
('Nyamagabe', 4),
('Nyanza', 4),
('Ruhango', 4),
('Nyaruguru', 4),

('Nyarugenge', 5),
('Kicukiro', 5),
('Gasabo', 5);
-- insert default departments
INSERT INTO departments (id,title) VALUES
(1,'ADMIN');
-- Insert a user with a hashed password
 INSERT INTO users (fname, lname, phone, email, can_add_codes,can_trigger_draw,can_add_user,can_view_logs, department_id, email_verified, phone_verified, locale, avatar_url, password, status, address, operator)
VALUES
('Admin', 'User', 'NOT_AVAILABLE', 'hirwa@hhlinks.rw', true, true, true, true, 1, FALSE, FALSE, 'en', 'NOT_AVAILABLE',
 '$2a$06$AJeg0ORjarDookEH7QO4iOpNKS2VEnUXdP2WuR6pH8Hu.XWIwGjNC', 'OKAY', 'NOT_AVAILABLE', NULL);