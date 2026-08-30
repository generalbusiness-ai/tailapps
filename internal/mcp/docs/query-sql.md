# Query SQL

`tailapp_query` runs bounded read-only SQL against one Tailapp's explicit
exports. `mounts` expose other Tailapps' exports as named aliases
(`{"cost": "session-cost"}` makes `cost.session_cost` queryable in the
same statement).

## The admitted subset

Single `SELECT` statements over exported relations: joins, `WHERE`,
`GROUP BY`, `ORDER BY`, aggregate functions, and positional `?` parameters
(pass values in `parameters`). No writes, no DDL, no PRAGMA, no attach, no
subqueries in disallowed positions, no multiple statements. `row_limit`
bounds the result (default and maximum 1000 rows); `truncated: true` marks
a clipped result.

## Result shape

`{tailapp, revision, delivery_head, interpreted_position,
ineffective_records, complete, columns, rows, result_bytes, truncated}`.
`columns` names and types the selected shape even when `rows` is empty.
`complete: false` or a rising `ineffective_records` means the projection
has not absorbed everything: check `tailapp_status` and
`tailapp_ineffective`.

## Value model

Values arrive as JSON: integers within the exactly representable range as
numbers, larger integers as `{"integer_decimal": "…"}`, blobs as
`{"bytes_base64": "…"}`, `BOOLEAN` columns as true/false, `JSON` columns
as decoded values.
