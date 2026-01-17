# SQL Validation (READ-only)

## Goal
Allow the main app to send SQL queries that are **read-only**.

## Hard rules
Reject any query containing (case-insensitive) keywords or patterns indicating writes or execution:
- INSERT, UPDATE, DELETE, MERGE
- TRUNCATE, DROP, ALTER, CREATE
- EXEC, EXECUTE
- GRANT, REVOKE
- ; (optional policy: reject multi-statements)
- Comments (optional policy: reject or strip safely)

Allow only:
- SELECT (including CTE/WITH + SELECT)
- ORDER BY, GROUP BY, HAVING, JOIN, UNION, OFFSET/FETCH

## Recommended approach (defense-in-depth)
1) Normalize whitespace and casing.
2) Reject multi-statement patterns (e.g., semicolons) unless explicitly allowed.
3) Use a SQL parser if available for the target dialect(s).
4) Fallback to strict keyword scanning with word-boundaries.
5) Enforce server-side limits:
   - timeout (e.g., 3–10 seconds)
   - max rows (e.g., 10k)
   - max response size

## Parameter binding
- All parameters must be passed separately in the request (`params` object).
- DB driver must bind parameters (no string concatenation).
