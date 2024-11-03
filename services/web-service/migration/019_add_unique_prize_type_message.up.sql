-- add unique constraint on prize_message table (lang and prize_type_id)
ALTER TABLE prize_message ADD CONSTRAINT unique_prize_message_lang_prize_type_id UNIQUE (lang, prize_type_id);
