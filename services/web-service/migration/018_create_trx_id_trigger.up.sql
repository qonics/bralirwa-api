-- Step 1: Create the trigger function
CREATE OR REPLACE FUNCTION update_transaction_trx_id()
RETURNS TRIGGER AS $$
BEGIN
    -- Check if trx_id is empty
    IF NEW.trx_id IS NULL OR NEW.trx_id = '' THEN
        -- Format the id as 7 digits with leading zeros and prepend "BRL"
        NEW.trx_id := 'BRL' || LPAD(NEW.id::text, 7, '0');
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Step 2: Create the trigger to call this function on INSERT
CREATE TRIGGER trg_update_transaction_trx_id
BEFORE INSERT ON transaction  -- Replace with your actual table name
FOR EACH ROW
EXECUTE FUNCTION update_transaction_trx_id();
