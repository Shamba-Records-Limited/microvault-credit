-- A second /r/{code} short-link, for MoneyGram's support deep-link.
--
-- It cannot share ramp_short_code, and not only because one column holds one
-- value: the two links have opposite lifetimes. The interactive webview link
-- must stop working once the withdrawal settles, so a leaked SMS cannot reopen
-- a completed session — the redirect handler 410s it. The support link is the
-- reverse: it matters precisely when the borrower is standing at an agent with
-- a problem, which is after settlement. Routing both through one column would
-- kill the support link exactly when it is needed.
--
-- Minted by the poller rather than at initiate, because MoneyGram only returns
-- more_info_url once the withdrawal is under way.
ALTER TABLE loans
    ADD COLUMN IF NOT EXISTS ramp_more_info_short_code VARCHAR(24);

CREATE UNIQUE INDEX IF NOT EXISTS idx_loans_ramp_more_info_short_code
    ON loans (ramp_more_info_short_code)
    WHERE ramp_more_info_short_code IS NOT NULL;
