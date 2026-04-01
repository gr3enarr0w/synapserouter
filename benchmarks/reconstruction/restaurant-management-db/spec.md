# Restaurant Management Database -- Reconstruction Spec

## Overview

A relational database for a multi-location restaurant business. The schema models the core operational domain: restaurants and their physical tables, staff scheduling, menu organization with categories, customer management, reservations, and order processing with line items. The database must support common reporting queries including daily revenue, popular menu items, staff shift coverage, and customer loyalty analysis.

Target dialect: PostgreSQL 15+ (use standard SQL where possible; PostgreSQL-specific features noted explicitly).

---

## Scope

### IN SCOPE

- 9 tables with full referential integrity (FK constraints, cascades)
- CHECK constraints for business rules (price > 0, capacity > 0, valid status enums)
- Seed data: minimum 5 rows per table, realistic values
- 30 query exercises spanning SELECT, JOIN, aggregation, subquery, window function, and CTE patterns
- 4 views for common reports
- Index recommendations for query workload
- Acceptance criteria with verifiable assertions

### OUT OF SCOPE

- Application code, ORM mappings, or API layer
- Payment processing, tax calculation, tip management
- Inventory/supply chain, ingredient-level tracking
- Multi-tenant isolation (all locations share one schema)
- Row-level security, roles, or permissions
- Stored procedures, triggers, or functions (queries only)
- NoSQL alternatives, denormalized analytics tables
- Migration tooling or versioning

---

## Database Schema

### 1. restaurants

| Column | Type | Constraints |
|--------|------|-------------|
| id | SERIAL | PRIMARY KEY |
| name | VARCHAR(100) | NOT NULL |
| address | VARCHAR(255) | NOT NULL |
| city | VARCHAR(100) | NOT NULL |
| state | VARCHAR(2) | NOT NULL |
| zip_code | VARCHAR(10) | NOT NULL |
| phone | VARCHAR(20) | NOT NULL |
| opening_date | DATE | NOT NULL |
| is_active | BOOLEAN | NOT NULL DEFAULT TRUE |

```sql
CREATE TABLE restaurants (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    address VARCHAR(255) NOT NULL,
    city VARCHAR(100) NOT NULL,
    state VARCHAR(2) NOT NULL,
    zip_code VARCHAR(10) NOT NULL,
    phone VARCHAR(20) NOT NULL,
    opening_date DATE NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE
);
```

### 2. tables

| Column | Type | Constraints |
|--------|------|-------------|
| id | SERIAL | PRIMARY KEY |
| restaurant_id | INTEGER | NOT NULL, FK -> restaurants(id) |
| table_number | INTEGER | NOT NULL |
| capacity | INTEGER | NOT NULL, CHECK (capacity > 0 AND capacity <= 20) |
| section | VARCHAR(50) | NOT NULL |
| is_available | BOOLEAN | NOT NULL DEFAULT TRUE |

```sql
CREATE TABLE tables (
    id SERIAL PRIMARY KEY,
    restaurant_id INTEGER NOT NULL REFERENCES restaurants(id) ON DELETE CASCADE,
    table_number INTEGER NOT NULL,
    capacity INTEGER NOT NULL CHECK (capacity > 0 AND capacity <= 20),
    section VARCHAR(50) NOT NULL,
    is_available BOOLEAN NOT NULL DEFAULT TRUE,
    UNIQUE (restaurant_id, table_number)
);
```

### 3. staff

| Column | Type | Constraints |
|--------|------|-------------|
| id | SERIAL | PRIMARY KEY |
| restaurant_id | INTEGER | NOT NULL, FK -> restaurants(id) |
| first_name | VARCHAR(50) | NOT NULL |
| last_name | VARCHAR(50) | NOT NULL |
| role | VARCHAR(30) | NOT NULL, CHECK IN ('manager', 'chef', 'sous_chef', 'server', 'bartender', 'host', 'busser') |
| hourly_rate | DECIMAL(6,2) | NOT NULL, CHECK (hourly_rate > 0) |
| hire_date | DATE | NOT NULL |
| is_active | BOOLEAN | NOT NULL DEFAULT TRUE |

```sql
CREATE TABLE staff (
    id SERIAL PRIMARY KEY,
    restaurant_id INTEGER NOT NULL REFERENCES restaurants(id) ON DELETE CASCADE,
    first_name VARCHAR(50) NOT NULL,
    last_name VARCHAR(50) NOT NULL,
    role VARCHAR(30) NOT NULL CHECK (role IN ('manager', 'chef', 'sous_chef', 'server', 'bartender', 'host', 'busser')),
    hourly_rate DECIMAL(6,2) NOT NULL CHECK (hourly_rate > 0),
    hire_date DATE NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE
);
```

### 4. categories

| Column | Type | Constraints |
|--------|------|-------------|
| id | SERIAL | PRIMARY KEY |
| name | VARCHAR(50) | NOT NULL UNIQUE |
| display_order | INTEGER | NOT NULL DEFAULT 0 |

```sql
CREATE TABLE categories (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE,
    display_order INTEGER NOT NULL DEFAULT 0
);
```

### 5. menu_items

| Column | Type | Constraints |
|--------|------|-------------|
| id | SERIAL | PRIMARY KEY |
| restaurant_id | INTEGER | NOT NULL, FK -> restaurants(id) |
| category_id | INTEGER | NOT NULL, FK -> categories(id) |
| name | VARCHAR(100) | NOT NULL |
| description | TEXT | |
| price | DECIMAL(8,2) | NOT NULL, CHECK (price > 0) |
| is_available | BOOLEAN | NOT NULL DEFAULT TRUE |
| created_at | TIMESTAMP | NOT NULL DEFAULT NOW() |

```sql
CREATE TABLE menu_items (
    id SERIAL PRIMARY KEY,
    restaurant_id INTEGER NOT NULL REFERENCES restaurants(id) ON DELETE CASCADE,
    category_id INTEGER NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    price DECIMAL(8,2) NOT NULL CHECK (price > 0),
    is_available BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
```

### 6. customers

| Column | Type | Constraints |
|--------|------|-------------|
| id | SERIAL | PRIMARY KEY |
| first_name | VARCHAR(50) | NOT NULL |
| last_name | VARCHAR(50) | NOT NULL |
| email | VARCHAR(100) | UNIQUE |
| phone | VARCHAR(20) | |
| loyalty_points | INTEGER | NOT NULL DEFAULT 0, CHECK (loyalty_points >= 0) |
| created_at | TIMESTAMP | NOT NULL DEFAULT NOW() |

```sql
CREATE TABLE customers (
    id SERIAL PRIMARY KEY,
    first_name VARCHAR(50) NOT NULL,
    last_name VARCHAR(50) NOT NULL,
    email VARCHAR(100) UNIQUE,
    phone VARCHAR(20),
    loyalty_points INTEGER NOT NULL DEFAULT 0 CHECK (loyalty_points >= 0),
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
```

### 7. reservations

| Column | Type | Constraints |
|--------|------|-------------|
| id | SERIAL | PRIMARY KEY |
| restaurant_id | INTEGER | NOT NULL, FK -> restaurants(id) |
| customer_id | INTEGER | NOT NULL, FK -> customers(id) |
| table_id | INTEGER | FK -> tables(id) |
| reservation_date | DATE | NOT NULL |
| reservation_time | TIME | NOT NULL |
| party_size | INTEGER | NOT NULL, CHECK (party_size > 0) |
| status | VARCHAR(20) | NOT NULL DEFAULT 'confirmed', CHECK IN ('confirmed', 'seated', 'completed', 'cancelled', 'no_show') |
| special_requests | TEXT | |
| created_at | TIMESTAMP | NOT NULL DEFAULT NOW() |

```sql
CREATE TABLE reservations (
    id SERIAL PRIMARY KEY,
    restaurant_id INTEGER NOT NULL REFERENCES restaurants(id) ON DELETE CASCADE,
    customer_id INTEGER NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    table_id INTEGER REFERENCES tables(id) ON DELETE SET NULL,
    reservation_date DATE NOT NULL,
    reservation_time TIME NOT NULL,
    party_size INTEGER NOT NULL CHECK (party_size > 0),
    status VARCHAR(20) NOT NULL DEFAULT 'confirmed' CHECK (status IN ('confirmed', 'seated', 'completed', 'cancelled', 'no_show')),
    special_requests TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
```

### 8. orders

| Column | Type | Constraints |
|--------|------|-------------|
| id | SERIAL | PRIMARY KEY |
| restaurant_id | INTEGER | NOT NULL, FK -> restaurants(id) |
| table_id | INTEGER | FK -> tables(id) |
| customer_id | INTEGER | FK -> customers(id) |
| server_id | INTEGER | FK -> staff(id) |
| order_time | TIMESTAMP | NOT NULL DEFAULT NOW() |
| status | VARCHAR(20) | NOT NULL DEFAULT 'open', CHECK IN ('open', 'in_progress', 'served', 'closed', 'cancelled') |
| total_amount | DECIMAL(10,2) | NOT NULL DEFAULT 0 CHECK (total_amount >= 0) |

```sql
CREATE TABLE orders (
    id SERIAL PRIMARY KEY,
    restaurant_id INTEGER NOT NULL REFERENCES restaurants(id) ON DELETE CASCADE,
    table_id INTEGER REFERENCES tables(id) ON DELETE SET NULL,
    customer_id INTEGER REFERENCES customers(id) ON DELETE SET NULL,
    server_id INTEGER REFERENCES staff(id) ON DELETE SET NULL,
    order_time TIMESTAMP NOT NULL DEFAULT NOW(),
    status VARCHAR(20) NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'in_progress', 'served', 'closed', 'cancelled')),
    total_amount DECIMAL(10,2) NOT NULL DEFAULT 0 CHECK (total_amount >= 0)
);
```

### 9. order_items

| Column | Type | Constraints |
|--------|------|-------------|
| id | SERIAL | PRIMARY KEY |
| order_id | INTEGER | NOT NULL, FK -> orders(id) ON DELETE CASCADE |
| menu_item_id | INTEGER | NOT NULL, FK -> menu_items(id) |
| quantity | INTEGER | NOT NULL, CHECK (quantity > 0) |
| unit_price | DECIMAL(8,2) | NOT NULL, CHECK (unit_price > 0) |
| special_instructions | TEXT | |

```sql
CREATE TABLE order_items (
    id SERIAL PRIMARY KEY,
    order_id INTEGER NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    menu_item_id INTEGER NOT NULL REFERENCES menu_items(id) ON DELETE RESTRICT,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    unit_price DECIMAL(8,2) NOT NULL CHECK (unit_price > 0),
    special_instructions TEXT
);
```

---

## Relationships (ER Description)

```
restaurants 1──* tables          (a restaurant has many tables)
restaurants 1──* staff           (a restaurant employs many staff)
restaurants 1──* menu_items      (a restaurant offers many menu items)
restaurants 1──* reservations    (a restaurant takes many reservations)
restaurants 1──* orders          (a restaurant processes many orders)

categories  1──* menu_items      (a category contains many menu items)

customers   1──* reservations    (a customer makes many reservations)
customers   1──* orders          (a customer places many orders)

tables      1──* reservations    (a table is reserved many times; nullable)
tables      1──* orders          (a table has many orders; nullable)

staff       1──* orders          (a server handles many orders; nullable)

orders      1──* order_items     (an order contains many line items)
menu_items  1──* order_items     (a menu item appears in many order line items)
```

Cascade rules:
- Deleting a restaurant cascades to tables, staff, menu_items, reservations, orders
- Deleting an order cascades to order_items
- Deleting a category is RESTRICTED if menu_items reference it
- Deleting a menu_item is RESTRICTED if order_items reference it
- Deleting a customer cascades to reservations; orders SET NULL
- Deleting a table or staff member: referencing orders/reservations SET NULL

---

## Seed Data

### restaurants (5 rows)

```sql
INSERT INTO restaurants (name, address, city, state, zip_code, phone, opening_date, is_active) VALUES
('The Golden Fork',    '123 Main St',     'Portland',  'OR', '97201', '503-555-0101', '2019-03-15', TRUE),
('Riverside Bistro',   '456 River Rd',    'Austin',    'TX', '78701', '512-555-0202', '2020-07-01', TRUE),
('Sakura Garden',      '789 Cherry Ln',   'Seattle',   'WA', '98101', '206-555-0303', '2018-11-20', TRUE),
('Ember & Vine',       '321 Oak Ave',     'Denver',    'CO', '80202', '303-555-0404', '2021-05-10', TRUE),
('Coastal Catch',      '654 Harbor Blvd',  'San Diego', 'CA', '92101', '619-555-0505', '2017-09-01', FALSE);
```

### tables (10 rows)

```sql
INSERT INTO tables (restaurant_id, table_number, capacity, section, is_available) VALUES
(1, 1,  2, 'patio',     TRUE),
(1, 2,  4, 'main',      TRUE),
(1, 3,  6, 'main',      FALSE),
(1, 4,  8, 'private',   TRUE),
(2, 1,  2, 'bar',       TRUE),
(2, 2,  4, 'main',      TRUE),
(2, 3,  4, 'main',      TRUE),
(3, 1,  2, 'sushi_bar', TRUE),
(3, 2,  4, 'main',      TRUE),
(3, 3,  6, 'tatami',    TRUE);
```

### staff (10 rows)

```sql
INSERT INTO staff (restaurant_id, first_name, last_name, role, hourly_rate, hire_date, is_active) VALUES
(1, 'Maria',   'Chen',      'manager',   32.00, '2019-03-15', TRUE),
(1, 'James',   'Wilson',    'chef',      28.00, '2019-04-01', TRUE),
(1, 'Sarah',   'Johnson',   'server',    18.50, '2020-06-15', TRUE),
(1, 'David',   'Kim',       'server',    18.50, '2021-01-10', TRUE),
(2, 'Elena',   'Rodriguez', 'manager',   30.00, '2020-07-01', TRUE),
(2, 'Marcus',  'Brown',     'chef',      26.00, '2020-08-01', TRUE),
(2, 'Lisa',    'Patel',     'server',    17.00, '2021-03-20', TRUE),
(3, 'Kenji',   'Tanaka',    'chef',      35.00, '2018-11-20', TRUE),
(3, 'Yuki',    'Sato',      'sous_chef', 25.00, '2019-02-01', TRUE),
(3, 'Amy',     'Nguyen',    'host',      16.00, '2020-01-15', FALSE);
```

### categories (7 rows)

```sql
INSERT INTO categories (name, display_order) VALUES
('Appetizers',  1),
('Salads',      2),
('Entrees',     3),
('Seafood',     4),
('Desserts',    5),
('Beverages',   6),
('Kids Menu',   7);
```

### menu_items (15 rows)

```sql
INSERT INTO menu_items (restaurant_id, category_id, name, description, price, is_available) VALUES
(1, 1, 'Truffle Fries',        'Hand-cut fries with truffle oil and parmesan',    12.50, TRUE),
(1, 3, 'Grilled Ribeye',       '14oz USDA Prime ribeye with herb butter',         42.00, TRUE),
(1, 5, 'Chocolate Lava Cake',  'Warm chocolate cake with molten center',          14.00, TRUE),
(1, 6, 'Craft IPA',            'Local brewery rotating IPA selection',             8.00, TRUE),
(2, 1, 'Queso Fundido',        'Melted cheese with chorizo and peppers',          11.00, TRUE),
(2, 3, 'BBQ Brisket Plate',    'Smoked 12-hour brisket with two sides',          24.00, TRUE),
(2, 2, 'Caesar Salad',         'Romaine, croutons, parmesan, house dressing',     13.00, TRUE),
(2, 6, 'Margarita',            'House margarita with fresh lime juice',            12.00, TRUE),
(3, 1, 'Edamame',              'Steamed soybeans with sea salt',                   6.50, TRUE),
(3, 4, 'Salmon Sashimi',       'Fresh Atlantic salmon, 8 pieces',                 18.00, TRUE),
(3, 3, 'Chicken Teriyaki',     'Grilled chicken with house teriyaki glaze',       19.00, TRUE),
(3, 4, 'Tuna Roll',            'Bluefin tuna, cucumber, avocado',                 16.00, FALSE),
(1, 2, 'Arugula Salad',        'Arugula, goat cheese, candied walnuts, balsamic', 14.00, TRUE),
(2, 5, 'Pecan Pie',            'Classic pecan pie with vanilla ice cream',         10.00, TRUE),
(1, 7, 'Kids Chicken Tenders', 'Breaded chicken tenders with fries',               9.00, TRUE);
```

### customers (8 rows)

```sql
INSERT INTO customers (first_name, last_name, email, phone, loyalty_points) VALUES
('Alice',   'Harper',    'alice.harper@email.com',   '503-555-1001', 450),
('Bob',     'Martinez',  'bob.martinez@email.com',   '512-555-1002', 120),
('Carol',   'Lee',       'carol.lee@email.com',      '206-555-1003', 890),
('Daniel',  'Wright',    'daniel.wright@email.com',   '303-555-1004', 50),
('Emma',    'Davis',     'emma.davis@email.com',       NULL,          0),
('Frank',   'Garcia',    'frank.garcia@email.com',    '619-555-1006', 320),
('Grace',   'Kim',       NULL,                        '503-555-1007', 200),
('Henry',   'Zhao',      'henry.zhao@email.com',      '206-555-1008', 1050);
```

### reservations (8 rows)

```sql
INSERT INTO reservations (restaurant_id, customer_id, table_id, reservation_date, reservation_time, party_size, status, special_requests) VALUES
(1, 1, 2, '2026-03-26', '18:30', 3, 'confirmed',  'Window seat preferred'),
(1, 3, 4, '2026-03-26', '19:00', 6, 'confirmed',  'Birthday celebration'),
(2, 2, 6, '2026-03-26', '19:30', 2, 'seated',     NULL),
(3, 8, 8, '2026-03-26', '18:00', 2, 'completed',  'Omakase tasting'),
(1, 7, 2, '2026-03-27', '20:00', 4, 'confirmed',  NULL),
(3, 3, 10,'2026-03-27', '19:00', 5, 'confirmed',  'Tatami room requested'),
(2, 4, 5, '2026-03-25', '18:00', 2, 'cancelled',  NULL),
(1, 1, 3, '2026-03-20', '19:00', 4, 'no_show',    NULL);
```

### orders (12 rows)

```sql
INSERT INTO orders (restaurant_id, table_id, customer_id, server_id, order_time, status, total_amount) VALUES
(1, 2, 1, 3, '2026-03-25 18:45:00', 'closed',      68.50),
(1, 3, 3, 4, '2026-03-25 19:15:00', 'closed',      112.00),
(1, 2, 7, 3, '2026-03-25 20:30:00', 'closed',      45.00),
(2, 6, 2, 7, '2026-03-25 19:00:00', 'closed',      60.00),
(2, 7, NULL, 7, '2026-03-25 20:00:00', 'closed',    37.00),
(3, 8, 8, NULL, '2026-03-25 18:15:00', 'closed',    52.50),
(3, 9, 3, NULL, '2026-03-25 19:30:00', 'closed',    73.00),
(1, 2, 1, 3, '2026-03-26 12:00:00', 'served',      26.50),
(1, 4, NULL, 4, '2026-03-26 12:30:00', 'in_progress', 42.00),
(2, 6, 2, 7, '2026-03-26 18:30:00', 'open',        0.00),
(3, 10,3, NULL, '2026-03-26 18:00:00', 'served',    34.00),
(1, 1, 5, 3, '2026-03-24 19:00:00', 'closed',      56.00);
```

### order_items (20 rows)

```sql
INSERT INTO order_items (order_id, menu_item_id, quantity, unit_price, special_instructions) VALUES
(1, 1,  1, 12.50, NULL),
(1, 2,  1, 42.00, 'Medium rare'),
(1, 5,  1, 14.00, NULL),
(2, 2,  2, 42.00, 'One rare, one medium'),
(2, 1,  1, 12.50, NULL),
(2, 4,  2,  8.00, NULL),
(3, 1,  1, 12.50, NULL),
(3, 13, 1, 14.00, 'No walnuts'),
(3, 4,  1,  8.00, NULL),
(4, 5,  1, 11.00, NULL),
(4, 6,  1, 24.00, 'Extra sauce'),
(4, 8,  2, 12.00, 'No salt rim'),
(5, 7,  1, 13.00, NULL),
(5, 6,  1, 24.00, NULL),
(6, 9,  1,  6.50, NULL),
(6, 10, 2, 18.00, NULL),
(6, 11, 1, 19.00, NULL),
(7, 11, 2, 19.00, NULL),
(7, 10, 1, 18.00, NULL),
(7, 9,  2,  6.50, NULL);
```

Note: `unit_price` is captured at time of order and may differ from current `menu_items.price` (price-at-sale pattern).

---

## Index Recommendations

```sql
-- Reservation lookups by date (common filter)
CREATE INDEX idx_reservations_date ON reservations (reservation_date);

-- Reservation lookups by customer
CREATE INDEX idx_reservations_customer ON reservations (customer_id);

-- Order lookups by date (revenue reports)
CREATE INDEX idx_orders_order_time ON orders (order_time);

-- Order lookups by restaurant and status (active orders dashboard)
CREATE INDEX idx_orders_restaurant_status ON orders (restaurant_id, status);

-- Order items by order (join acceleration)
CREATE INDEX idx_order_items_order ON order_items (order_id);

-- Menu items by restaurant and category (menu display)
CREATE INDEX idx_menu_items_restaurant_category ON menu_items (restaurant_id, category_id);

-- Staff by restaurant (scheduling)
CREATE INDEX idx_staff_restaurant ON staff (restaurant_id);

-- Customer email lookup (login/search)
-- Already UNIQUE, so implicit index exists on customers.email
```

---

## Views

### 1. v_daily_revenue

Daily revenue per restaurant from closed orders.

```sql
CREATE VIEW v_daily_revenue AS
SELECT
    r.id AS restaurant_id,
    r.name AS restaurant_name,
    DATE(o.order_time) AS order_date,
    COUNT(o.id) AS order_count,
    SUM(o.total_amount) AS daily_total
FROM orders o
JOIN restaurants r ON r.id = o.restaurant_id
WHERE o.status = 'closed'
GROUP BY r.id, r.name, DATE(o.order_time);
```

### 2. v_popular_items

Menu items ranked by total quantity sold across all orders.

```sql
CREATE VIEW v_popular_items AS
SELECT
    mi.id AS menu_item_id,
    mi.name AS item_name,
    c.name AS category_name,
    r.name AS restaurant_name,
    SUM(oi.quantity) AS total_quantity_sold,
    SUM(oi.quantity * oi.unit_price) AS total_revenue
FROM order_items oi
JOIN menu_items mi ON mi.id = oi.menu_item_id
JOIN categories c ON c.id = mi.category_id
JOIN orders o ON o.id = oi.order_id
JOIN restaurants r ON r.id = mi.restaurant_id
WHERE o.status IN ('closed', 'served')
GROUP BY mi.id, mi.name, c.name, r.name;
```

### 3. v_staff_directory

Active staff with restaurant assignment.

```sql
CREATE VIEW v_staff_directory AS
SELECT
    s.id AS staff_id,
    s.first_name || ' ' || s.last_name AS full_name,
    s.role,
    s.hourly_rate,
    s.hire_date,
    r.name AS restaurant_name,
    r.city
FROM staff s
JOIN restaurants r ON r.id = s.restaurant_id
WHERE s.is_active = TRUE
ORDER BY r.name, s.role, s.last_name;
```

### 4. v_reservation_board

Today's and future reservations with customer and table info.

```sql
CREATE VIEW v_reservation_board AS
SELECT
    res.id AS reservation_id,
    rest.name AS restaurant_name,
    c.first_name || ' ' || c.last_name AS customer_name,
    c.phone AS customer_phone,
    t.table_number,
    t.section,
    t.capacity AS table_capacity,
    res.reservation_date,
    res.reservation_time,
    res.party_size,
    res.status,
    res.special_requests
FROM reservations res
JOIN restaurants rest ON rest.id = res.restaurant_id
JOIN customers c ON c.id = res.customer_id
LEFT JOIN tables t ON t.id = res.table_id
WHERE res.reservation_date >= CURRENT_DATE
ORDER BY res.reservation_date, res.reservation_time;
```

---

## Query Exercises

### Basic SELECT (1-5)

**Q1.** List all active restaurants with their city and state, ordered by opening date (oldest first).

Expected: 4 rows (Coastal Catch is inactive). Columns: name, city, state, opening_date.

**Q2.** Find all menu items priced above $20, showing name, price, and restaurant name.

Expected: 2 rows (Grilled Ribeye at $42.00, BBQ Brisket Plate at $24.00).

**Q3.** List all customers who have more than 300 loyalty points, ordered by points descending.

Expected: 3 rows (Henry 1050, Carol 890, Alice 450).

**Q4.** Show all unavailable menu items (is_available = FALSE) with their category name.

Expected: 1 row (Tuna Roll, Seafood).

**Q5.** List all tables in the 'main' section across all restaurants, showing restaurant name, table number, and capacity.

Expected: 5 rows.

---

### JOINs (6-12)

**Q6.** For each order, show the order ID, restaurant name, server's full name, and order status. Include orders with no assigned server (show NULL).

Expected: 12 rows. Orders 6, 7, 11 have NULL server.

**Q7.** List all reservations for March 26, 2026, showing customer name, restaurant name, table number, party size, and status.

Expected: 4 rows.

**Q8.** Show every menu item with its category name and restaurant name, ordered by restaurant then category display_order then item name.

Expected: 15 rows.

**Q9.** List all order items for order #2, showing item name, quantity, unit price, and line total (quantity * unit_price).

Expected: 3 rows (Grilled Ribeye x2 = 84.00, Truffle Fries x1 = 12.50, Craft IPA x2 = 16.00). Total should equal 112.50.

**Q10.** Show all customers who have placed at least one order, with the number of orders per customer. Use LEFT JOIN from customers to orders so customers with no orders still appear (showing 0).

Expected: 8 customer rows. Emma has 1 order, Daniel has 0, Frank has 0.

**Q11.** List all restaurants and the count of active staff members per restaurant. Include restaurants with no active staff (Coastal Catch, Ember & Vine).

Expected: 5 rows. Coastal Catch and Ember & Vine show 0.

**Q12.** Find all reservations where the party size exceeds the assigned table's capacity.

Expected: 0 or more rows depending on data. With seed data: reservation #2 (party_size=6, table_id=4 capacity=8) fits. Verify all assignments.

---

### Aggregations (13-18)

**Q13.** Calculate total revenue per restaurant from closed orders only.

Expected: 3 restaurants with revenue. The Golden Fork: 68.50+112.00+45.00+56.00 = 281.50, Riverside Bistro: 60.00+37.00 = 97.00, Sakura Garden: 52.50+73.00 = 125.50.

**Q14.** Find the average menu item price per category, ordered by average price descending.

Expected: 7 categories. Entrees should have highest average, Kids Menu lowest.

**Q15.** Count the number of orders per day for the last 7 days, including days with zero orders.

Expected: Use generate_series to fill gaps. Only dates with orders in seed data: 2026-03-24, 2026-03-25, 2026-03-26.

**Q16.** Find the most expensive menu item at each restaurant.

Expected: 3 rows (one per active restaurant with menu items). The Golden Fork: Grilled Ribeye $42.00.

**Q17.** Calculate the total quantity of each menu item sold, showing only items with more than 2 total units sold.

Expected: Items where SUM(quantity) > 2. Check: Truffle Fries (3), Grilled Ribeye (3), Craft IPA (3), Chicken Teriyaki (3), Salmon Sashimi (3).

**Q18.** Show the number of reservations per status type.

Expected: confirmed: 3, seated: 1, completed: 1, cancelled: 1, no_show: 1 (if 7 total seed reservations have those statuses -- verify against seed data: confirmed=3, seated=1, completed=1, cancelled=1, no_show=1).

---

### Subqueries (19-22)

**Q19.** Find all customers whose total spending (sum of order total_amount from closed orders) exceeds the average customer spending.

Expected: Calculate average across all customers with closed orders, then filter above that.

**Q20.** List menu items that have never been ordered (not present in any order_item).

Expected: Items with IDs not in order_items. Check: Kids Chicken Tenders (id=15), Pecan Pie (id=14), Margarita/Caesar/Queso already ordered. Tuna Roll (id=12) not ordered. Arugula Salad (id=13) is ordered in order 3.

**Q21.** Find the restaurant with the highest single-day revenue from closed orders.

Expected: Use a subquery to calculate daily revenue per restaurant, then select the max.

**Q22.** List all servers who have handled more orders than the average number of orders per server.

Expected: Calculate AVG orders per server, then find servers above that threshold.

---

### Window Functions (23-27)

**Q23.** Rank menu items by price within each category using RANK(). Show category name, item name, price, and rank.

Expected: 15 rows with ranks. Items at same price within a category share rank.

**Q24.** For each order, show the order ID, order_time, total_amount, and a running total of revenue per restaurant (ordered by order_time).

Expected: 12 rows. Running total resets per restaurant.

**Q25.** Show each staff member's hourly rate and the difference from the average rate at their restaurant.

Expected: 10 rows. Positive means above average, negative means below.

**Q26.** For each customer, show their orders with the previous order's total_amount using LAG(), ordered by order_time.

Expected: NULL for first order per customer.

**Q27.** Assign each menu item a percentile rank (PERCENT_RANK) by price within its restaurant.

Expected: 15 rows. Most expensive item at each restaurant = 1.0, cheapest = 0.0.

---

### CTEs (28-30)

**Q28.** Using a CTE, find the top 3 menu items by total revenue (quantity * unit_price) across all orders. Show item name, restaurant, total revenue, and percentage of grand total revenue.

Expected: 3 rows. Grilled Ribeye likely first.

**Q29.** Using a recursive CTE is not applicable here (no hierarchical data). Instead, use a multi-step CTE: first CTE calculates per-customer spending, second CTE calculates spending tiers (under $50 = 'bronze', $50-$100 = 'silver', over $100 = 'gold'). Show customer name, total spent, and tier.

Expected: All customers with at least one closed order get a tier. Customers with no closed orders excluded or shown as 'bronze' with $0.

**Q30.** Using a CTE, find for each restaurant: the busiest hour (hour with most orders), the most popular category (by items sold), and total revenue. Combine into a single restaurant summary row.

Expected: One row per restaurant with three derived metrics.

---

## Acceptance Criteria

### Schema

- [ ] All 9 tables created without errors
- [ ] All SERIAL primary keys auto-increment
- [ ] All foreign key constraints enforced: inserting an order_item with a nonexistent order_id fails
- [ ] CHECK constraints enforced: inserting a menu_item with price = 0 fails
- [ ] CHECK constraints enforced: inserting a staff member with role = 'janitor' fails
- [ ] UNIQUE constraint on (restaurant_id, table_number) prevents duplicate table numbers within a restaurant
- [ ] UNIQUE constraint on categories.name prevents duplicate category names
- [ ] UNIQUE constraint on customers.email allows NULL (multiple customers without email permitted)
- [ ] CASCADE: deleting restaurant id=5 removes its tables, staff, menu_items, reservations, orders
- [ ] RESTRICT: deleting category 'Appetizers' fails while menu_items reference it
- [ ] SET NULL: deleting a staff member sets orders.server_id to NULL

### Seed Data

- [ ] All INSERT statements execute without constraint violations
- [ ] restaurants: 5 rows (4 active, 1 inactive)
- [ ] tables: 10 rows across 3 restaurants
- [ ] staff: 10 rows (9 active, 1 inactive)
- [ ] categories: 7 rows
- [ ] menu_items: 15 rows (14 available, 1 unavailable)
- [ ] customers: 8 rows
- [ ] reservations: 8 rows covering all 5 status values
- [ ] orders: 12 rows covering all 5 status values
- [ ] order_items: 20 rows

### Views

- [ ] v_daily_revenue returns correct daily totals for closed orders only
- [ ] v_popular_items shows quantity and revenue per item
- [ ] v_staff_directory shows only active staff
- [ ] v_reservation_board shows only today and future reservations

### Queries

- [ ] All 30 query exercises are answerable with the provided schema and seed data
- [ ] Queries 1-5 (basic SELECT) require no JOINs
- [ ] Queries 6-12 (JOINs) use at least 2 tables each
- [ ] Queries 13-18 (aggregations) use GROUP BY with aggregate functions
- [ ] Queries 19-22 (subqueries) use at least one subquery (correlated or uncorrelated)
- [ ] Queries 23-27 (window functions) use at least one window function (RANK, ROW_NUMBER, LAG, PERCENT_RANK, or SUM OVER)
- [ ] Queries 28-30 (CTEs) use WITH clause
- [ ] Expected result counts in exercise descriptions match actual query results against seed data

### Indexes

- [ ] All recommended indexes created without errors
- [ ] Indexes cover the primary query patterns (date range, status filter, FK joins)
