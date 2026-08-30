package mcp

// The tool catalog: metadata for every tool - titles, three-clause
// descriptions with stable docs pointers, input and output schemas in plain
// JSON Schema 2020-12, and complete behavior annotations. The SDK server
// registers exactly this catalog; the resource pages derive from it.
func tools() []tool {
	object := func(properties map[string]any, required ...string) map[string]any {
		if properties == nil {
			properties = map[string]any{}
		}
		schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
		if len(required) > 0 {
			schema["required"] = required
		}
		return schema
	}
	// Result schemas are plain JSON Schema 2020-12 with no $schema marker (a
	// foreign dialect disables the whole catalog on Ajv-validating clients).
	// They stay open to additional properties so the engine contract can grow
	// without breaking validating clients; required names the stable core.
	result := func(properties map[string]any, required ...string) map[string]any {
		schema := map[string]any{"type": "object", "properties": properties}
		if len(required) > 0 {
			schema["required"] = required
		}
		return schema
	}
	text := map[string]any{"type": "string"}
	integer := map[string]any{"type": "integer"}
	number := map[string]any{"type": "number"}
	boolean := map[string]any{"type": "boolean"}
	anyObject := map[string]any{"type": "object"}
	anyArray := map[string]any{"type": "array"}
	idempotencyKey := map[string]any{"type": "string", "minLength": 1, "maxLength": 128, "pattern": `^[!-~]+$`}
	sourceContent := map[string]any{"type": "string", "contentEncoding": "base64"}

	appResult := result(map[string]any{"name": text, "draft_revision": text, "active_revision": text, "runtime_profile": text, "activation_mode": text, "boundary_position": integer}, "name", "draft_revision")
	frontierResult := result(map[string]any{"revision": text, "activation_boundary": integer, "interpreted_position": integer, "last_event_id": text, "complete": boolean, "gap_position": integer, "gap_reason": text}, "revision", "complete")
	profileResult := result(map[string]any{"name": text, "revision": text, "runtime_profile": text, "storage_schema_digest": text, "export_contract_digest": text, "event": anyObject, "normalizer": anyObject, "folds": anyArray, "tables": anyObject, "views": anyObject, "exports": anyObject, "schema_sql": anyArray}, "name", "revision")

	install := object(map[string]any{
		"name": text, "bundle": text,
		"sources":         map[string]any{"type": "object", "additionalProperties": sourceContent, "minProperties": 1},
		"idempotency_key": idempotencyKey,
	}, "name", "idempotency_key")
	install["oneOf"] = []map[string]any{{"required": []string{"bundle"}}, {"required": []string{"sources"}}}

	// Safety hints, stated completely (see the annotations type). The
	// draftWrite tools never destroy existing materialized state: three edit
	// drafts under optimistic revision control, and tailapp_install
	// first-activates a brand-new Tailapp (touching live state additively,
	// create-only). The two destructive tools discard existing state.
	readOnly := annotations{ReadOnlyHint: true, DestructiveHint: false, IdempotentHint: false, OpenWorldHint: false}
	draftWrite := annotations{ReadOnlyHint: false, DestructiveHint: false, IdempotentHint: true, OpenWorldHint: false}
	destructive := annotations{ReadOnlyHint: false, DestructiveHint: true, IdempotentHint: true, OpenWorldHint: false}

	return []tool{
		{Name: "tailapps_list", Title: "List Tailapps", Annotations: readOnly,
			Description:  "List every local Tailapp with its draft and active revisions. Returns {\"apps\": [...]}; an empty engine returns {\"apps\": []}, never null. Start here, then read data with tailapp_query. docs: tailapp://docs/tools/tailapps_list",
			InputSchema:  object(nil),
			OutputSchema: result(map[string]any{"apps": map[string]any{"type": "array", "items": appResult}}, "apps")},
		{Name: "tailapp_get", Title: "Read a Tailapp draft", Annotations: readOnly,
			Description:  "Read one Tailapp's definition and complete source map at its exact draft revision. Returns {app, sources} with base64 source values; a just-created empty draft returns {} sources. Use the returned draft_revision as expected_revision for draft edits; draft edits are not live until tailapp_activate. docs: tailapp://docs/tools/tailapp_get",
			InputSchema:  object(map[string]any{"name": text}, "name"),
			OutputSchema: result(map[string]any{"app": appResult, "sources": anyObject}, "app", "sources")},
		{Name: "tailapp_create", Title: "Create a Tailapp draft", Annotations: draftWrite,
			Description:  "Create a new Tailapp draft, empty or copied from a built-in bundle. A draft only: nothing runs until validated and activated. Returns the app; a fresh empty draft has only name and draft_revision, no active fields. Follow with tailapp_put_element, then tailapp_validate and tailapp_activate; replaying the same idempotency_key returns the original outcome. docs: tailapp://docs/tools/tailapp_create",
			InputSchema:  object(map[string]any{"name": text, "bundle": text, "idempotency_key": idempotencyKey}, "name", "idempotency_key"),
			OutputSchema: appResult},
		{Name: "tailapp_install", Title: "Install a Tailapp", Annotations: draftWrite,
			Description:  "Validate a complete source set and first-activate one new Tailapp in a single request, from either a built-in bundle or a base64 source map (exactly one). Create-only: an existing name is refused, never replaced, and there is no partial success - failure installs nothing. Returns {app, profile, frontier} for the activated app. The one-step alternative to the create/put/validate/activate sequence; replaying the same idempotency_key returns the original outcome. docs: tailapp://docs/tools/tailapp_install",
			InputSchema:  install,
			OutputSchema: result(map[string]any{"app": appResult, "profile": map[string]any{"type": []string{"object", "null"}}, "frontier": frontierResult}, "app", "frontier")},
		{Name: "tailapp_delete", Title: "Delete a Tailapp", Annotations: destructive,
			Description:  "Delete one Tailapp definition and detach only its projection; the materialized analytics leave live query reach. Returns {deleted: true}, its only shape - an unknown name is an error, not an empty result. The end of a Tailapp's lifecycle; replaying the same idempotency_key returns the original outcome. docs: tailapp://docs/tools/tailapp_delete",
			InputSchema:  object(map[string]any{"name": text, "idempotency_key": idempotencyKey}, "name", "idempotency_key"),
			OutputSchema: result(map[string]any{"deleted": boolean}, "deleted")},
		{Name: "tailapp_put_element", Title: "Write a draft element", Annotations: draftWrite,
			Description:  "Write one bounded source element (application.sql or folds/*.jsonata, base64) into a Tailapp draft under optimistic revision control: expected_revision must match the current draft. Returns the app with its new draft_revision and no other change - the draft is not live until tailapp_activate. Sits between tailapp_create and tailapp_validate in the draft loop. docs: tailapp://docs/tools/tailapp_put_element",
			InputSchema:  object(map[string]any{"name": text, "path": text, "content": map[string]any{"type": "string", "contentEncoding": "base64"}, "expected_revision": text, "idempotency_key": idempotencyKey}, "name", "path", "content", "expected_revision", "idempotency_key"),
			OutputSchema: appResult},
		{Name: "tailapp_delete_element", Title: "Delete a draft element", Annotations: draftWrite,
			Description:  "Remove one source element from a Tailapp draft under optimistic revision control (expected_revision). Returns the app with its new draft_revision; removing the last element leaves a valid empty draft. Draft-only, between create and validate; not live until tailapp_activate. docs: tailapp://docs/tools/tailapp_delete_element",
			InputSchema:  object(map[string]any{"name": text, "path": text, "expected_revision": text, "idempotency_key": idempotencyKey}, "name", "path", "expected_revision", "idempotency_key"),
			OutputSchema: appResult},
		{Name: "tailapp_validate", Title: "Validate a draft", Annotations: readOnly,
			Description:  "Compile the exact draft at expected_revision without changing live behavior. Returns the full compiled profile (identity digests, event and table schemas, exports); a draft that does not compile returns the diagnostic as an error result, never a partial profile. Run before tailapp_activate. docs: tailapp://docs/tools/tailapp_validate",
			InputSchema:  object(map[string]any{"name": text, "expected_revision": text}, "name", "expected_revision"),
			OutputSchema: profileResult},
		{Name: "tailapp_activate", Title: "Activate a draft", Annotations: destructive,
			Description:  "Activate a validated draft at a delivery boundary. First activation and mode reset discard materialized state and require acknowledge_reset true; mode continue preserves tables across compatible revisions. Returns the projection {frontier}; a healthy new frontier has complete true and no gap fields. The last step of the draft loop; replaying the same idempotency_key returns the original outcome. docs: tailapp://docs/tools/tailapp_activate",
			InputSchema:  object(map[string]any{"name": text, "expected_revision": text, "mode": map[string]any{"type": "string", "enum": []string{"continue", "reset"}}, "acknowledge_reset": boolean, "idempotency_key": idempotencyKey}, "name", "expected_revision", "mode", "idempotency_key"),
			OutputSchema: frontierResult},
		{Name: "tailapp_status", Title: "Engine status", Annotations: readOnly,
			Description:  "Read engine readiness, inbox bounds, and every Tailapp's exact projection frontier and gaps. Returns {profile, ingestion_ready, inbox, apps, unavailable}; with no Tailapps installed, apps is {}. Start here when telemetry seems missing, before tailapp_ineffective. docs: tailapp://docs/tools/tailapp_status",
			InputSchema:  object(nil),
			OutputSchema: result(map[string]any{"profile": text, "ingestion_ready": boolean, "inbox": anyObject, "apps": anyObject, "unavailable": anyObject}, "profile", "ingestion_ready", "inbox", "apps")},
		{Name: "tailapp_metrics", Title: "Runtime metrics", Annotations: readOnly,
			Description:  "Read the versioned, payload-free performance snapshot. Returns a flat object of counters and gauges with per-Tailapp processing stats ({version, inbox, tailapps, ...}); counters are cumulative for the resident lifetime - a fresh never-used resident shows an empty tailapps map and zero intake activity while uptime and runtime gauges are already nonzero - and no field ever carries telemetry content. Pair with tailapp_status for operational triage. docs: tailapp://docs/tools/tailapp_metrics",
			InputSchema:  object(nil),
			OutputSchema: result(map[string]any{"version": text, "reset_semantics": text, "started_at": text, "generated_at": text, "uptime_seconds": number, "inbox": anyObject, "tailapps": anyObject, "active_tailapps": integer, "unavailable_tailapps": integer, "upgrade_pending_tailapps": integer, "omitted_tailapps": integer}, "version", "inbox", "tailapps")},
		{Name: "tailapp_ineffective", Title: "Inspect rejected records", Annotations: readOnly,
			Description:  "Inspect the bounded, memory-only buffer of recent canonical records one Tailapp's normalizer rejected, for adapter-shape diagnosis. Returns {tailapp, revision, capacity, ineffective_records, records}; a Tailapp with no rejections returns records: [] with ineffective_records 0. Records can contain sensitive telemetry: read locally, share aggregates. Use after tailapp_status shows intake but a query stays empty. docs: tailapp://docs/tools/tailapp_ineffective",
			InputSchema:  object(map[string]any{"name": text}, "name"),
			OutputSchema: result(map[string]any{"tailapp": text, "revision": text, "capacity": integer, "ineffective_records": integer, "available_records": integer, "unavailable_records": integer, "records": anyArray}, "tailapp", "revision", "capacity", "records")},
		{Name: "tailapp_schema", Title: "Read a Tailapp schema", Annotations: readOnly,
			Description:  "Read one active Tailapp's compiled shape: private tables and their writers, the event schema, and the explicit exports queryable through tailapp_query. Returns the compiled profile object; derived from compiled source, never from telemetry, so it is stable between activations. Read before writing SQL for tailapp_query. docs: tailapp://docs/tools/tailapp_schema",
			InputSchema:  object(map[string]any{"name": text}, "name"),
			OutputSchema: profileResult},
		{Name: "tailapp_query", Title: "Run read-only SQL", Annotations: readOnly,
			Description:  "Run bounded read-only SQL against one Tailapp's explicit exports; mounts expose other Tailapps' exports as named aliases. Detective observation, never inline prevention. Returns {columns, rows, complete, truncated} with the exact projection position; an empty projection returns rows: [] with columns still describing the selected shape, not an error. Results derive from local telemetry and may identify sessions: prefer aggregates when sharing. The main read tool, after tailapps_list. docs: tailapp://docs/tools/tailapp_query",
			InputSchema:  object(map[string]any{"name": text, "sql": text, "parameters": map[string]any{"type": "array", "maxItems": 64}, "mounts": map[string]any{"type": "object", "additionalProperties": text}, "expected_revision": text, "expected_position": map[string]any{"type": "integer"}, "row_limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 1000}}, "name", "sql"),
			OutputSchema: result(map[string]any{"tailapp": text, "revision": text, "delivery_head": integer, "interpreted_position": integer, "ineffective_records": integer, "schemas": anyArray, "complete": boolean, "columns": anyArray, "rows": anyArray, "result_bytes": integer, "truncated": boolean}, "tailapp", "revision", "complete", "columns", "rows")},
	}
}
