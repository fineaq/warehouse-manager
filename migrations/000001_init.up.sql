CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(50) NOT NULL,
    email TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    timezone TEXT NOT NULL DEFAULT 'Asia/Jakarta'
);

CREATE TABLE users (
    id UUID DEFAULT gen_random_uuid(),
    name VARCHAR(50) NOT NULL,
    tenant_id UUID NOT NULL REFERENCES tenants (id),
    email TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (tenant_id, email),
    PRIMARY KEY (tenant_id, id)
);

CREATE TABLE locations (
    id UUID DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants (id),
    name VARCHAR(50) NOT NULL,
    code TEXT NOT NULL,
    address TEXT,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (tenant_id, code),
    PRIMARY KEY (tenant_id, id)
);

CREATE TABLE products (
    id UUID DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants (id),
    name VARCHAR(50) NOT NULL,
    code TEXT NOT NULL,
    unit VARCHAR(100) NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (tenant_id, code),
    PRIMARY KEY (tenant_id, id)
);

CREATE TABLE stock_movements (
    id UUID DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    location_id UUID NOT NULL,
    product_id UUID NOT NULL,
    user_id UUID NOT NULL,
    quantity NUMERIC(14,3) NOT NULL,
    reason VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    note TEXT,

    CHECK (quantity <> 0),
    CHECK (reason IN ('receive','issue','transfer_in','transfer_out','adjust','dispose','count')),
    FOREIGN KEY (tenant_id, location_id) REFERENCES locations (tenant_id, id),
    FOREIGN KEY (tenant_id, product_id) REFERENCES products (tenant_id, id),
    FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id),
    PRIMARY KEY (tenant_id, id)
);

CREATE TABLE stock_balances (
    tenant_id UUID NOT NULL,
    location_id UUID NOT NULL,
    product_id UUID NOT NULL,
    on_hand NUMERIC(14,3) NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CHECK (on_hand >= 0),
    PRIMARY KEY (tenant_id, location_id, product_id),
    FOREIGN KEY (tenant_id, location_id) REFERENCES locations (tenant_id, id),
    FOREIGN KEY (tenant_id, product_id)  REFERENCES products  (tenant_id, id)
);

CREATE INDEX idx_stock_movements_position
  ON stock_movements (tenant_id, location_id, product_id, created_at);
CREATE INDEX idx_stock_balances_product ON stock_balances (tenant_id, product_id);
