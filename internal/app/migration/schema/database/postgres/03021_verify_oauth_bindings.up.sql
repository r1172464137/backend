-- OAuth sign-in refuses an unverified binding: one asserted without proof of
-- the identity must not open the account to whoever proves it later. Bindings
-- administrators created before the flag was set on their behalf are stored
-- unverified although the administrator vouched for them, so those accounts
-- cannot sign in through the provider; mark them verified. Email, mobile and
-- device bindings are proven by their own codes and are left as they are.
UPDATE "user_auth_methods"
SET "verified" = true
WHERE "auth_type" NOT IN ('email', 'mobile', 'device')
  AND "verified" = false;
