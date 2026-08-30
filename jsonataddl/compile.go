package jsonataddl

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	jsonata "github.com/jsonata-go/jsonata/v206"
	"github.com/ncruces/go-sqlite3"
	sqlitedriver "github.com/ncruces/go-sqlite3/driver"
)

// evaluationWallTimeMilliseconds is only an outer process-safety net for one
// JSONata evaluation. It is part of the evaluator component of runtime
// identity; admitted programs are confined statically, and wall time is not
// a deterministic evaluation budget.
const evaluationWallTimeMilliseconds = 250

var (
	createEventRE       = regexp.MustCompile(`(?is)^CREATE\s+EVENT\s+([A-Za-z_][A-Za-z0-9_]*)\s*\((.*)\)$`)
	createTableRE       = regexp.MustCompile(`(?is)^CREATE\s+TABLE\s+([A-Za-z_][A-Za-z0-9_]*)\s*\((.*)\)$`)
	createIndexRE       = regexp.MustCompile(`(?is)^CREATE\s+(?:UNIQUE\s+)?INDEX\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	createViewRE        = regexp.MustCompile(`(?is)^CREATE\s+VIEW\s+([A-Za-z_][A-Za-z0-9_]*)\s+AS\s+(.+)$`)
	createExportRE      = regexp.MustCompile(`(?is)^CREATE\s+EXPORT\s+([A-Za-z_][A-Za-z0-9_]*)\s+AS\s+(.+)$`)
	createNormalizerRE  = regexp.MustCompile(`(?is)^CREATE\s+NORMALIZER\s+([A-Za-z_][A-Za-z0-9_]*)\s+ON\s+([A-Za-z_][A-Za-z0-9_]*)\s+(.+)$`)
	createFoldRE        = regexp.MustCompile(`(?is)^CREATE\s+FOLD\s+([A-Za-z_][A-Za-z0-9_]*)\s+ON\s+([A-Za-z_][A-Za-z0-9_]*)\s+(.+)$`)
	readStartRE         = regexp.MustCompile(`(?is)\bREAD\s+([A-Za-z_][A-Za-z0-9_]*)\s+(ONE|OPTIONAL\s+ONE|MANY\s+LIMIT\s+[0-9]+)\s+AS\s+`)
	usingRE             = regexp.MustCompile(`(?is)\bUSING\s+'([^']+)'\s+(.+)$`)
	normalizerTailRE    = regexp.MustCompile(`(?is)^(?:WRITES\s+(.+?)\s+)?EMITS\s+([A-Za-z_][A-Za-z0-9_]*)$`)
	foldTailRE          = regexp.MustCompile(`(?is)^WRITES\s+(.+)$`)
	primaryKeyRE        = regexp.MustCompile(`(?is)^PRIMARY\s+KEY\s*\(([^)]+)\)$`)
	uniqueKeyRE         = regexp.MustCompile(`(?is)^UNIQUE\s*\(([^)]+)\)$`)
	selectRE            = regexp.MustCompile(`(?is)^SELECT\s+(.+?)\s+FROM\s+([A-Za-z_][A-Za-z0-9_]*)(?:\s+WHERE\s+(.+?))?(?:\s+ORDER\s+BY\s+(.+))?$`)
	whereTermRE         = regexp.MustCompile(`(?is)^([A-Za-z_][A-Za-z0-9_]*)\s*=\s*:event\.([A-Za-z_][A-Za-z0-9_]*)$`)
	parameterRE         = regexp.MustCompile(`:event\.([A-Za-z_][A-Za-z0-9_]*)`)
	relationRE          = regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	ambientSQLRE        = regexp.MustCompile(`(?i)\b(?:random|randomblob|current_date|current_time|current_timestamp|load_extension)\b|\b(?:date|time|datetime|julianday|unixepoch|strftime)\s*\(`)
	commaJoinRE         = regexp.MustCompile(`(?is)\b(?:FROM|JOIN)\s+[A-Za-z_][A-Za-z0-9_]*(?:\s+(?:AS\s+)?[A-Za-z_][A-Za-z0-9_]*)?\s*,`)
	forbiddenSQLRE      = regexp.MustCompile(`(?i)\b(?:ALTER|DROP|TRIGGER|VIRTUAL|PRAGMA|ATTACH|DETACH|VACUUM|REINDEX|ANALYZE|INSERT|UPDATE|DELETE|REPLACE)\b`)
	forbiddenQueryRE    = regexp.MustCompile(`(?i);|\b(?:WITH|UNION|INTERSECT|EXCEPT|PRAGMA|ATTACH|DETACH|INSERT|UPDATE|DELETE|REPLACE|CREATE|ALTER|DROP)\b|\(\s*SELECT\b`)
	functionCallRE      = regexp.MustCompile(`(?i)\b[A-Za-z_][A-Za-z0-9_]*\s*\(`)
	unsupportedClauseRE = regexp.MustCompile(`\b(?:DEFAULT|GENERATED|AUTOINCREMENT|COLLATE|REFERENCES)\b`)
	starSelectRE        = regexp.MustCompile(`(?i)SELECT\s+\*|,\s*\*`)
	joinRE              = regexp.MustCompile(`(?i)\bJOIN\b`)
	andSplitRE          = regexp.MustCompile(`(?i)\s+AND\s+`)
)

type coreCompiler struct {
	app          *Application
	dialect      Dialect
	sources      map[string][]byte
	programNames map[string]bool
	boundSources map[string]bool
	viewSQL      map[string]string
}

// validateDialectPolicy admits only the policies this core implements.
// Parameterization is not permission to build an arbitrary dataflow graph:
// the closed two-stage topology is the only one the compiler and evaluator
// implement, and any other request is refused rather than half-honored.
func validateDialectPolicy(dialect Dialect) error {
	if dialect.HostEvent.Name == "" || dialect.PrivateEvent.Name == "" {
		return errors.New("dialect must name the host and private events")
	}
	if dialect.HostEvent.Name == dialect.PrivateEvent.Name {
		return errors.New("dialect host and private events must be distinct")
	}
	if !dialect.PrivateEvent.ExactlyOne {
		return errors.New("dialect private-event policy is outside the implemented topology")
	}
	if !dialect.Topology.ExactlyOneNormalizer || !dialect.Topology.AtLeastOneFold || dialect.Topology.FoldsMayEmitEvents {
		return errors.New("dialect topology policy is outside the implemented topology")
	}
	if !dialect.Authority.SingleWriterTables {
		return errors.New("dialect authority policy requires single-writer tables")
	}
	switch dialect.Authority.NormalizerReads {
	case ReadOwnTables, ReadOwnAndNormalizerTables:
	default:
		return fmt.Errorf("dialect normalizer read visibility %q is not implemented", dialect.Authority.NormalizerReads)
	}
	switch dialect.Authority.FoldReads {
	case ReadOwnTables, ReadOwnAndNormalizerTables:
	default:
		return fmt.Errorf("dialect fold read visibility %q is not implemented", dialect.Authority.FoldReads)
	}
	limits := dialect.Limits
	for _, bound := range []int{limits.MaxElementBytes, limits.MaxSourceBytes, limits.MaxProgramBytes,
		limits.MaxInputBytes, limits.MaxOutputBytes, limits.MaxDepth, limits.MaxRange,
		limits.MaxEvents, limits.MaxFacts, limits.MaxRowChanges, limits.MaxManyRows} {
		if bound < 1 {
			return errors.New("dialect limits must all be positive")
		}
	}
	for _, field := range dialect.HostEvent.Fields() {
		if !identifierRE.MatchString(field.Name) {
			return fmt.Errorf("dialect host-event field %q is not an identifier", field.Name)
		}
	}
	return nil
}

func compileApplication(name string, sources map[string][]byte, dialect Dialect, runtimeProfile string) (*Application, error) {
	if err := validateDialectPolicy(dialect); err != nil {
		return nil, err
	}
	copySources := make(map[string][]byte, len(sources))
	for key, value := range sources {
		copySources[key] = append([]byte(nil), value...)
	}
	compiler := &coreCompiler{
		app: &Application{
			name:           name,
			runtimeProfile: runtimeProfile,
			dialect:        dialect,
			tables:         make(map[string]Table),
			views:          make(map[string]View),
			exports:        make(map[string]Export),
			sources:        copySources,
		},
		dialect:      dialect,
		sources:      sources,
		programNames: make(map[string]bool),
		boundSources: make(map[string]bool),
		viewSQL:      make(map[string]string),
	}
	if err := compiler.compile(); err != nil {
		return nil, err
	}
	return compiler.app, nil
}

func (c *coreCompiler) compile() error {
	statements, err := splitDDLStatements(string(c.sources[c.dialect.Layout.DefinitionPath]))
	if err != nil {
		return err
	}
	if len(statements) == 0 {
		return fmt.Errorf("%s has no declarations", c.dialect.Layout.DefinitionPath)
	}
	var exportQueries = make(map[string]string)
	for _, statement := range statements {
		if forbiddenSQLRE.MatchString(statement) {
			return fmt.Errorf("statement contains forbidden SQL: %.64q", statement)
		}
		switch {
		case createEventRE.MatchString(statement):
			if err := c.compileEvent(statement); err != nil {
				return err
			}
		case createTableRE.MatchString(statement):
			if ambientSQLRE.MatchString(statement) {
				return errors.New("table uses an ambient SQL function")
			}
			if err := c.compileTable(statement); err != nil {
				return err
			}
		case createIndexRE.MatchString(statement):
			if ambientSQLRE.MatchString(statement) {
				return errors.New("index uses an ambient SQL function")
			}
			c.app.replaceableSQL = append(c.app.replaceableSQL, statement)
		case createViewRE.MatchString(statement):
			match := createViewRE.FindStringSubmatch(statement)
			if err := validateAdmittedQuery(match[2], true); err != nil {
				return fmt.Errorf("view %q: %w", match[1], err)
			}
			if c.nameExists(match[1]) {
				return fmt.Errorf("schema name %q is declared twice", match[1])
			}
			dependencies := queryRelationNames(match[2])
			if len(dependencies) == 0 {
				return fmt.Errorf("view %q has no relation", match[1])
			}
			c.viewSQL[match[1]] = match[2]
			c.app.views[match[1]] = View{Name: match[1], SQL: strings.TrimSpace(match[2]), Dependencies: dependencies}
			c.app.replaceableSQL = append(c.app.replaceableSQL, statement)
		case createExportRE.MatchString(statement):
			match := createExportRE.FindStringSubmatch(statement)
			if err := validateAdmittedQuery(match[2], true); err != nil {
				return fmt.Errorf("export %q: %w", match[1], err)
			}
			if _, exists := exportQueries[match[1]]; exists {
				return fmt.Errorf("export %q is declared twice", match[1])
			}
			exportQueries[match[1]] = strings.TrimSpace(match[2])
		case createNormalizerRE.MatchString(statement):
			if c.app.normalizer.Name != "" {
				return errors.New("application declares more than one normalizer")
			}
			match := createNormalizerRE.FindStringSubmatch(statement)
			program, err := c.compileProgram(match[1], match[2], match[3], true)
			if err != nil {
				return err
			}
			c.app.normalizer = program
		case createFoldRE.MatchString(statement):
			match := createFoldRE.FindStringSubmatch(statement)
			program, err := c.compileProgram(match[1], match[2], match[3], false)
			if err != nil {
				return err
			}
			c.app.folds = append(c.app.folds, program)
		default:
			return fmt.Errorf("statement is outside the application DDL profile: %.64q", statement)
		}
	}
	if err := c.validateTopology(); err != nil {
		return err
	}
	database, err := c.buildScratch()
	if err != nil {
		return err
	}
	defer database.Close()
	if err := c.validateReads(database); err != nil {
		return err
	}
	if err := c.compileExports(database, exportQueries); err != nil {
		return err
	}
	if err := c.computeDigests(); err != nil {
		return err
	}
	for source := range c.sources {
		if source != c.dialect.Layout.DefinitionPath && !c.boundSources[source] {
			return fmt.Errorf("JSONata source %q is not bound by a declaration", source)
		}
	}
	return nil
}

func (c *coreCompiler) nameExists(name string) bool {
	for table := range c.app.tables {
		if strings.EqualFold(table, name) {
			return true
		}
	}
	for view := range c.viewSQL {
		if strings.EqualFold(view, name) {
			return true
		}
	}
	return false
}

func (c *coreCompiler) compileEvent(statement string) error {
	if c.app.event.Name != "" {
		return errors.New("application declares more than one event")
	}
	match := createEventRE.FindStringSubmatch(statement)
	if match[1] != c.dialect.PrivateEvent.Name {
		return fmt.Errorf("private event must be named %s, got %q", c.dialect.PrivateEvent.Name, match[1])
	}
	columns, _, _, err := parseColumnList(match[2], false)
	if err != nil {
		return fmt.Errorf("event %s: %w", match[1], err)
	}
	c.app.event = Event{Name: match[1], Columns: columns}
	return nil
}

func (c *coreCompiler) compileTable(statement string) error {
	match := createTableRE.FindStringSubmatch(statement)
	name := match[1]
	if c.nameExists(name) {
		return fmt.Errorf("schema name %q is declared twice", name)
	}
	columns, primary, unique, err := parseColumnList(match[2], true)
	if err != nil {
		return fmt.Errorf("table %q: %w", name, err)
	}
	if len(primary) == 0 {
		return fmt.Errorf("table %q requires an explicit primary key", name)
	}
	unique = append([][]string{append([]string(nil), primary...)}, unique...)
	c.app.tables[name] = Table{
		Name: name, Columns: columns, PrimaryKey: primary, UniqueKeys: unique,
		StorageShape: normalizeSQLTokens(statement), SQL: statement,
	}
	c.app.schemaSQL = append(c.app.schemaSQL, statement)
	return nil
}

func parseColumnList(body string, table bool) ([]Column, []string, [][]string, error) {
	parts, err := splitTopLevelComma(body)
	if err != nil {
		return nil, nil, nil, err
	}
	var columns []Column
	var primary []string
	var unique [][]string
	seen := make(map[string]bool)
	for _, part := range parts {
		if match := primaryKeyRE.FindStringSubmatch(strings.TrimSpace(part)); match != nil {
			if !table || len(primary) != 0 {
				return nil, nil, nil, errors.New("duplicate or invalid table primary key")
			}
			primary, err = identifierList(match[1])
			if err != nil {
				return nil, nil, nil, err
			}
			continue
		}
		if match := uniqueKeyRE.FindStringSubmatch(strings.TrimSpace(part)); match != nil {
			if !table {
				return nil, nil, nil, errors.New("event cannot declare a table constraint")
			}
			key, keyErr := identifierList(match[1])
			if keyErr != nil {
				return nil, nil, nil, keyErr
			}
			unique = append(unique, key)
			continue
		}
		fields := strings.Fields(part)
		if len(fields) < 2 || !identifierRE.MatchString(fields[0]) {
			return nil, nil, nil, fmt.Errorf("unsupported column declaration %q", part)
		}
		name := fields[0]
		key := strings.ToLower(name)
		if seen[key] {
			return nil, nil, nil, fmt.Errorf("column %q is declared twice", name)
		}
		seen[key] = true
		logical := LogicalType(strings.ToUpper(fields[1]))
		if !validCoreLogicalType(logical) {
			return nil, nil, nil, fmt.Errorf("column %q has unsupported type %q", name, fields[1])
		}
		upper := strings.ToUpper(" " + strings.Join(fields[2:], " ") + " ")
		if unsupportedClauseRE.MatchString(upper) {
			return nil, nil, nil, fmt.Errorf("column %q uses an unsupported clause", name)
		}
		inlinePrimary := strings.Contains(upper, " PRIMARY KEY ")
		if inlinePrimary {
			if !table || len(primary) != 0 {
				return nil, nil, nil, errors.New("duplicate or invalid primary key")
			}
			primary = []string{name}
		}
		columns = append(columns, Column{Name: name, Type: logical, NotNull: strings.Contains(upper, " NOT NULL ") || inlinePrimary, PrimaryKey: inlinePrimary})
	}
	if len(columns) == 0 {
		return nil, nil, nil, errors.New("declaration has no columns")
	}
	for _, key := range append([][]string{primary}, unique...) {
		for _, name := range key {
			if !seen[strings.ToLower(name)] {
				return nil, nil, nil, fmt.Errorf("key names undeclared column %q", name)
			}
		}
	}
	for i := range columns {
		for _, name := range primary {
			if strings.EqualFold(columns[i].Name, name) {
				columns[i].PrimaryKey = true
			}
		}
	}
	return columns, primary, unique, nil
}

func validCoreLogicalType(value LogicalType) bool {
	switch value {
	case TypeText, TypeInteger, TypeReal, TypeBlob, TypeBoolean, TypeJSON:
		return true
	}
	return false
}

func identifierList(value string) ([]string, error) {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]bool)
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if !identifierRE.MatchString(name) {
			return nil, fmt.Errorf("invalid identifier %q", name)
		}
		key := strings.ToLower(name)
		if seen[key] {
			return nil, fmt.Errorf("duplicate identifier %q", name)
		}
		seen[key] = true
		result = append(result, name)
	}
	if len(result) == 0 {
		return nil, errors.New("empty identifier list")
	}
	return result, nil
}

func (c *coreCompiler) compileProgram(name, event, body string, normalizer bool) (Program, error) {
	if c.programNames[name] {
		return Program{}, fmt.Errorf("program %q is declared twice", name)
	}
	c.programNames[name] = true
	using := usingRE.FindStringSubmatch(body)
	if using == nil {
		return Program{}, fmt.Errorf("program %q has no valid USING clause", name)
	}
	prefix := body[:strings.Index(strings.ToUpper(body), "USING")]
	reads, consumed, err := compileReadClauses(prefix, c.dialect.Limits.MaxManyRows)
	if err != nil {
		return Program{}, fmt.Errorf("program %q: %w", name, err)
	}
	if strings.TrimSpace(consumed) != "" {
		return Program{}, fmt.Errorf("program %q has unsupported text before USING", name)
	}
	programPath := using[1]
	if err := validateSourcePath(programPath); err != nil || !layoutAdmits(c.dialect.Layout, programPath) || programPath == c.dialect.Layout.DefinitionPath {
		return Program{}, fmt.Errorf("program %q has invalid source path %q", name, programPath)
	}
	source, exists := c.sources[programPath]
	if !exists {
		return Program{}, fmt.Errorf("program %q source %q does not exist", name, programPath)
	}
	if len(source) > c.dialect.Limits.MaxProgramBytes {
		return Program{}, fmt.Errorf("program %q exceeds %d bytes", name, c.dialect.Limits.MaxProgramBytes)
	}
	c.boundSources[programPath] = true
	if ambientJSONataRE.Match(source) {
		return Program{}, fmt.Errorf("program %q uses an ambient or dynamic JSONata function", name)
	}
	if err := validateJSONataLexicalSource(source); err != nil {
		return Program{}, fmt.Errorf("program %q: %w", name, err)
	}
	expression, err := jsonata.Compile(string(source), false)
	if err != nil {
		return Program{}, fmt.Errorf("compile program %q: %w", name, err)
	}
	if err := validateJSONataAST(expression.AST()); err != nil {
		return Program{}, fmt.Errorf("program %q: %w", name, err)
	}
	expression.SetMaxDepth(c.dialect.Limits.MaxDepth)
	expression.SetMaxRange(c.dialect.Limits.MaxRange)
	expression.SetMaxTime(evaluationWallTimeMilliseconds)
	program := Program{Name: name, Event: event, Path: programPath, Reads: reads, Normalizer: normalizer, expression: &compiledExpression{expression: expression}}
	if normalizer {
		if event != c.dialect.HostEvent.Name {
			return Program{}, fmt.Errorf("normalizer %q must consume %s", name, c.dialect.HostEvent.Name)
		}
		tail := normalizerTailRE.FindStringSubmatch(strings.TrimSpace(using[2]))
		if tail == nil {
			return Program{}, fmt.Errorf("normalizer %q must end with optional WRITES and EMITS", name)
		}
		if tail[1] != "" {
			program.Writes, err = identifierList(tail[1])
			if err != nil {
				return Program{}, err
			}
		}
		program.Emits = tail[2]
		if program.Emits != c.dialect.PrivateEvent.Name {
			return Program{}, fmt.Errorf("normalizer %q may emit only %s", name, c.dialect.PrivateEvent.Name)
		}
	} else {
		if event != c.dialect.PrivateEvent.Name {
			return Program{}, fmt.Errorf("analytic fold %q must consume %s", name, c.dialect.PrivateEvent.Name)
		}
		tail := foldTailRE.FindStringSubmatch(strings.TrimSpace(using[2]))
		if tail == nil {
			return Program{}, fmt.Errorf("analytic fold %q must end with WRITES", name)
		}
		program.Writes, err = identifierList(tail[1])
		if err != nil {
			return Program{}, err
		}
	}
	return program, nil
}

func compileReadClauses(prefix string, maxManyRows int) ([]Read, string, error) {
	var reads []Read
	spans := readStartRE.FindAllStringSubmatchIndex(prefix, -1)
	if len(spans) == 0 {
		return nil, prefix, nil
	}
	consumed := prefix[:spans[0][0]]
	seen := make(map[string]bool)
	for index, span := range spans {
		name := prefix[span[2]:span[3]]
		if seen[strings.ToLower(name)] {
			return nil, consumed, fmt.Errorf("read %q is declared twice", name)
		}
		seen[strings.ToLower(name)] = true
		cardinalityText := strings.Join(strings.Fields(strings.ToUpper(prefix[span[4]:span[5]])), " ")
		queryEnd := len(prefix)
		if index+1 < len(spans) {
			queryEnd = spans[index+1][0]
		}
		query := strings.TrimSpace(prefix[span[1]:queryEnd])
		read, err := compileReadQuery(name, cardinalityText, query, maxManyRows)
		if err != nil {
			return nil, consumed, fmt.Errorf("read %q: %w", name, err)
		}
		reads = append(reads, read)
	}
	return reads, consumed, nil
}

func compileReadQuery(name, cardinalityText, query string, maxManyRows int) (Read, error) {
	if err := validateAdmittedQuery(query, false); err != nil {
		return Read{}, err
	}
	match := selectRE.FindStringSubmatch(strings.TrimSpace(query))
	if match == nil {
		return Read{}, errors.New("read must be a simple named-column SELECT over one relation")
	}
	columns, err := identifierList(match[1])
	if err != nil {
		return Read{}, fmt.Errorf("selected columns: %w", err)
	}
	read := Read{Name: name, SQL: query, Table: match[2], Columns: columns}
	switch {
	case cardinalityText == "ONE":
		read.Cardinality = One
	case cardinalityText == "OPTIONAL ONE":
		read.Cardinality = OptionalOne
	case strings.HasPrefix(cardinalityText, "MANY LIMIT "):
		read.Cardinality = Many
		read.Limit, err = strconv.Atoi(strings.TrimPrefix(cardinalityText, "MANY LIMIT "))
		if err != nil || read.Limit < 1 || read.Limit > maxManyRows {
			return Read{}, fmt.Errorf("MANY limit must be between 1 and %d", maxManyRows)
		}
	default:
		return Read{}, fmt.Errorf("unsupported cardinality %q", cardinalityText)
	}
	if match[3] == "" && read.Cardinality != Many {
		return Read{}, errors.New("ONE reads require an event-key WHERE clause")
	}
	if match[3] != "" {
		terms := andSplitRE.Split(match[3], -1)
		for _, term := range terms {
			termMatch := whereTermRE.FindStringSubmatch(strings.TrimSpace(term))
			if termMatch == nil {
				return Read{}, errors.New("WHERE terms must be column = :event.field joined by AND")
			}
			read.Parameters = append(read.Parameters, termMatch[2])
		}
	}
	if match[4] != "" {
		for _, term := range strings.Split(match[4], ",") {
			fields := strings.Fields(strings.TrimSpace(term))
			if len(fields) < 1 || len(fields) > 2 || !identifierRE.MatchString(fields[0]) {
				return Read{}, errors.New("ORDER BY accepts only named columns with optional ASC or DESC")
			}
			if len(fields) == 2 && !strings.EqualFold(fields[1], "ASC") && !strings.EqualFold(fields[1], "DESC") {
				return Read{}, errors.New("invalid ORDER BY direction")
			}
			read.OrderBy = append(read.OrderBy, fields[0])
		}
	}
	if read.Cardinality == Many && len(read.OrderBy) == 0 {
		return Read{}, errors.New("MANY reads require a total ORDER BY")
	}
	read.SQL = parameterRE.ReplaceAllString(query, "?")
	if read.Cardinality == Many {
		read.SQL += fmt.Sprintf(" LIMIT %d", read.Limit)
	}
	return read, nil
}

func validateAdmittedQuery(query string, allowJoins bool) error {
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 16<<10 {
		return errors.New("query is empty or too large")
	}
	if ambientSQLRE.MatchString(query) {
		return errors.New("query uses an ambient SQL function")
	}
	if forbiddenQueryRE.MatchString(query) {
		return errors.New("query uses syntax outside the admitted SELECT profile")
	}
	if starSelectRE.MatchString(query) {
		return errors.New("SELECT * is not allowed")
	}
	if functionCallRE.MatchString(query) {
		return errors.New("SQL functions are not allowed in application queries")
	}
	withoutParameters := parameterRE.ReplaceAllString(query, "?")
	if strings.Contains(withoutParameters, ".") {
		return errors.New("qualified or cross-namespace relations are not allowed")
	}
	if !allowJoins && joinRE.MatchString(query) {
		return errors.New("fold reads may name only one relation")
	}
	if commaJoinRE.MatchString(query) {
		return errors.New("comma-separated joins are outside the admitted SELECT profile")
	}
	return nil
}

func (c *coreCompiler) validateTopology() error {
	if c.app.event.Name != c.dialect.PrivateEvent.Name {
		return fmt.Errorf("application requires exactly one private %s", c.dialect.PrivateEvent.Name)
	}
	if c.app.normalizer.Name == "" {
		return errors.New("application requires exactly one normalizer")
	}
	if len(c.app.folds) == 0 {
		return errors.New("application requires at least one analytic fold")
	}
	eventFields := make(map[string]bool)
	for _, column := range c.app.event.Columns {
		if column.Type != TypeJSON {
			eventFields[column.Name] = true
		}
	}
	envelopeFields := make(map[string]bool)
	for _, field := range c.dialect.HostEvent.Fields() {
		envelopeFields[field.Name] = true
	}
	writers := make(map[string]string)
	programs := append([]Program{c.app.normalizer}, c.app.folds...)
	for programIndex := range programs {
		program := &programs[programIndex]
		for _, tableName := range program.Writes {
			table, exists := c.app.tables[tableName]
			if !exists {
				return fmt.Errorf("program %q writes undeclared table %q", program.Name, tableName)
			}
			if prior, exists := writers[tableName]; exists {
				return fmt.Errorf("table %q has multiple writers %q and %q", tableName, prior, program.Name)
			}
			writers[tableName] = program.Name
			table.Writer = program.Name
			c.app.tables[tableName] = table
		}
		fields := eventFields
		if program.Normalizer {
			fields = envelopeFields
		}
		for _, read := range program.Reads {
			for _, parameter := range read.Parameters {
				if !fields[parameter] {
					return fmt.Errorf("program %q read %q names undeclared scalar event field %q", program.Name, read.Name, parameter)
				}
			}
		}
	}
	for tableName, table := range c.app.tables {
		if table.Writer == "" {
			return fmt.Errorf("table %q has no declared writer", tableName)
		}
	}
	for viewName := range c.app.views {
		if _, err := c.resolveView(viewName, nil); err != nil {
			return err
		}
	}
	for programIndex := range programs {
		program := programs[programIndex]
		visibility := c.dialect.Authority.FoldReads
		if program.Normalizer {
			visibility = c.dialect.Authority.NormalizerReads
		}
		for _, read := range program.Reads {
			baseTables, err := c.resolveRelation(read.Table)
			if err != nil {
				return fmt.Errorf("program %q read %q: %w", program.Name, read.Name, err)
			}
			for _, table := range baseTables {
				if err := c.checkReadVisibility(program, visibility, read.Table, table); err != nil {
					return err
				}
			}
			if read.Cardinality == Many {
				table, direct := c.app.tables[read.Table]
				if !direct || !orderEndsInUniqueKey(read.OrderBy, table.UniqueKeys) {
					return fmt.Errorf("program %q read %q ORDER BY must end in a declared unique key", program.Name, read.Name)
				}
			}
		}
	}
	return nil
}

// checkReadVisibility enforces the dialect's read-authority rule for one
// base table behind one read relation. The diagnostics carry the program
// role so a violation reads the same whether visibility came from the
// normalizer rule or the fold rule.
func (c *coreCompiler) checkReadVisibility(program Program, visibility ReadVisibility, relation string, table Table) error {
	switch visibility {
	case ReadOwnTables:
		if table.Writer != program.Name {
			if program.Normalizer {
				return fmt.Errorf("normalizer %q cannot read relation %q backed by table %q owned by %q", program.Name, relation, table.Name, table.Writer)
			}
			return fmt.Errorf("analytic fold %q cannot read relation %q backed by table %q owned by %q", program.Name, relation, table.Name, table.Writer)
		}
	case ReadOwnAndNormalizerTables:
		if table.Writer != c.app.normalizer.Name && table.Writer != program.Name {
			if program.Normalizer {
				return fmt.Errorf("normalizer %q cannot read relation %q backed by table %q owned by %q", program.Name, relation, table.Name, table.Writer)
			}
			return fmt.Errorf("analytic fold %q cannot read relation %q backed by table %q owned by analytic fold %q", program.Name, relation, table.Name, table.Writer)
		}
	default:
		return fmt.Errorf("read visibility %q is not implemented", visibility)
	}
	return nil
}

func queryRelationNames(query string) []string {
	matches := relationRE.FindAllStringSubmatch(query, -1)
	seen := make(map[string]bool)
	var result []string
	for _, match := range matches {
		key := strings.ToLower(match[1])
		if !seen[key] {
			seen[key] = true
			result = append(result, match[1])
		}
	}
	return result
}

func (c *coreCompiler) resolveRelation(name string) ([]Table, error) {
	if table, exists := c.app.tables[name]; exists {
		return []Table{table}, nil
	}
	return c.resolveView(name, nil)
}

func (c *coreCompiler) resolveView(name string, stack map[string]bool) ([]Table, error) {
	view, exists := c.app.views[name]
	if !exists {
		return nil, fmt.Errorf("relation %q is undeclared", name)
	}
	if stack == nil {
		stack = make(map[string]bool)
	}
	key := strings.ToLower(name)
	if stack[key] {
		return nil, fmt.Errorf("view dependency cycle at %q", name)
	}
	stack[key] = true
	defer delete(stack, key)
	seen := make(map[string]bool)
	var tables []Table
	for _, dependency := range view.Dependencies {
		if table, exists := c.app.tables[dependency]; exists {
			if !seen[strings.ToLower(table.Name)] {
				seen[strings.ToLower(table.Name)] = true
				tables = append(tables, table)
			}
			continue
		}
		resolved, err := c.resolveView(dependency, stack)
		if err != nil {
			return nil, fmt.Errorf("view %q: %w", name, err)
		}
		for _, table := range resolved {
			if !seen[strings.ToLower(table.Name)] {
				seen[strings.ToLower(table.Name)] = true
				tables = append(tables, table)
			}
		}
	}
	return tables, nil
}

func orderEndsInUniqueKey(order []string, keys [][]string) bool {
	for _, key := range keys {
		if len(key) > len(order) {
			continue
		}
		start := len(order) - len(key)
		match := true
		for i := range key {
			if !strings.EqualFold(order[start+i], key[i]) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func (c *coreCompiler) buildScratch() (*sql.DB, error) {
	dsn := fmt.Sprintf("file:jsonataddl-compile-%d?mode=memory&cache=private", time.Now().UnixNano())
	database, err := sqlitedriver.Open(dsn, func(connection *sqlite3.Conn) error {
		if _, err := connection.Config(sqlite3.DBCONFIG_DEFENSIVE, true); err != nil {
			return err
		}
		if _, err := connection.Config(sqlite3.DBCONFIG_TRUSTED_SCHEMA, false); err != nil {
			return err
		}
		if _, err := connection.Config(sqlite3.DBCONFIG_ENABLE_LOAD_EXTENSION, false); err != nil {
			return err
		}
		return connection.Exec("PRAGMA foreign_keys=ON")
	})
	if err != nil {
		return nil, fmt.Errorf("open compiler scratch database: %w", err)
	}
	database.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for _, statement := range append(append([]string(nil), c.app.schemaSQL...), c.app.replaceableSQL...) {
		if _, err := database.ExecContext(ctx, statement); err != nil {
			database.Close()
			return nil, fmt.Errorf("apply application schema: %w", err)
		}
	}
	return database, nil
}

func (c *coreCompiler) validateReads(database *sql.DB) error {
	programs := append([]Program{c.app.normalizer}, c.app.folds...)
	for _, program := range programs {
		for _, read := range program.Reads {
			args := make([]any, len(read.Parameters))
			query := "SELECT * FROM (" + read.SQL + ") LIMIT 0"
			rows, err := database.Query(query, args...)
			if err != nil {
				return fmt.Errorf("program %q read %q does not prepare: %w", program.Name, read.Name, err)
			}
			columns, columnErr := rows.Columns()
			rows.Close()
			if columnErr != nil {
				return columnErr
			}
			if duplicateColumnName(columns) != "" {
				return fmt.Errorf("program %q read %q has duplicate output column %q", program.Name, read.Name, duplicateColumnName(columns))
			}
		}
	}
	return nil
}

func (c *coreCompiler) compileExports(database *sql.DB, queries map[string]string) error {
	for _, name := range sortedNames(queries) {
		query := queries[name]
		for _, relation := range queryRelationNames(query) {
			if c.isView(relation) {
				return fmt.Errorf("export %q reads application view %q; export base tables directly in this profile", name, relation)
			}
		}
		rows, err := database.Query("SELECT * FROM (" + query + ") LIMIT 0")
		if err != nil {
			return fmt.Errorf("export %q does not prepare: %w", name, err)
		}
		columnTypes, err := rows.ColumnTypes()
		rows.Close()
		if err != nil {
			return fmt.Errorf("export %q columns: %w", name, err)
		}
		columns := make([]ExportColumn, 0, len(columnTypes))
		seen := make(map[string]bool)
		for _, columnType := range columnTypes {
			columnName := columnType.Name()
			if columnName == "" || seen[strings.ToLower(columnName)] {
				return fmt.Errorf("export %q has empty or duplicate output column %q", name, columnName)
			}
			seen[strings.ToLower(columnName)] = true
			logical := LogicalType(strings.ToUpper(columnType.DatabaseTypeName()))
			if !validCoreLogicalType(logical) {
				return fmt.Errorf("export %q column %q has no fixed logical type", name, columnName)
			}
			columns = append(columns, ExportColumn{Name: columnName, Type: logical})
		}
		contract, _ := json.Marshal(columns)
		c.app.exports[name] = Export{Name: name, SQL: query, Columns: columns, ContractDigest: digestEncoded(contract)}
	}
	if len(c.app.exports) == 0 {
		return errors.New("application requires at least one query export")
	}
	return nil
}

func (c *coreCompiler) isView(name string) bool {
	for candidate := range c.app.views {
		if strings.EqualFold(candidate, name) {
			return true
		}
	}
	return false
}

func (c *coreCompiler) computeDigests() error {
	tables := make([]Table, 0, len(c.app.tables))
	for _, name := range sortedNames(c.app.tables) {
		table := c.app.tables[name]
		table.SQL = ""
		table.Writer = ""
		tables = append(tables, table)
	}
	storage, err := json.Marshal(tables)
	if err != nil {
		return err
	}
	c.app.storageSchemaDigest = digestEncoded(storage)
	exports := make([]Export, 0, len(c.app.exports))
	for _, name := range sortedNames(c.app.exports) {
		exports = append(exports, c.app.exports[name])
	}
	contract, err := json.Marshal(exports)
	if err != nil {
		return err
	}
	c.app.exportContractDigest = digestEncoded(contract)
	return nil
}

func duplicateColumnName(names []string) string {
	seen := make(map[string]bool)
	for _, name := range names {
		key := strings.ToLower(name)
		if seen[key] {
			return name
		}
		seen[key] = true
	}
	return ""
}

func splitDDLStatements(source string) ([]string, error) {
	var statements []string
	var current strings.Builder
	quote := rune(0)
	lineComment, blockComment := false, false
	runes := []rune(source)
	for index := 0; index < len(runes); index++ {
		char := runes[index]
		next := rune(0)
		if index+1 < len(runes) {
			next = runes[index+1]
		}
		switch {
		case lineComment:
			if char == '\n' {
				lineComment = false
				current.WriteRune(char)
			}
		case blockComment:
			if char == '*' && next == '/' {
				blockComment = false
				index++
				current.WriteByte(' ')
			}
		case quote != 0:
			current.WriteRune(char)
			if char == quote {
				if next == quote {
					current.WriteRune(next)
					index++
				} else {
					quote = 0
				}
			}
		case char == '-' && next == '-':
			lineComment = true
			index++
		case char == '/' && next == '*':
			blockComment = true
			index++
		case char == '\'' || char == '"':
			quote = char
			current.WriteRune(char)
		case char == ';':
			if statement := strings.TrimSpace(current.String()); statement != "" {
				statements = append(statements, statement)
			}
			current.Reset()
		default:
			current.WriteRune(char)
		}
	}
	if quote != 0 || blockComment {
		return nil, errors.New("application SQL has an unterminated quote or comment")
	}
	if statement := strings.TrimSpace(current.String()); statement != "" {
		return nil, errors.New("application SQL statement is missing its semicolon")
	}
	return statements, nil
}

func splitTopLevelComma(source string) ([]string, error) {
	var parts []string
	var current strings.Builder
	depth := 0
	quote := rune(0)
	for _, char := range source {
		switch {
		case quote != 0:
			current.WriteRune(char)
			if char == quote {
				quote = 0
			}
		case char == '\'' || char == '"':
			quote = char
			current.WriteRune(char)
		case char == '(':
			depth++
			current.WriteRune(char)
		case char == ')':
			depth--
			if depth < 0 {
				return nil, errors.New("unbalanced parentheses")
			}
			current.WriteRune(char)
		case char == ',' && depth == 0:
			parts = append(parts, strings.TrimSpace(current.String()))
			current.Reset()
		default:
			current.WriteRune(char)
		}
	}
	if quote != 0 || depth != 0 {
		return nil, errors.New("unbalanced declaration")
	}
	parts = append(parts, strings.TrimSpace(current.String()))
	for _, part := range parts {
		if part == "" || strings.IndexFunc(part, unicode.IsControl) >= 0 {
			return nil, errors.New("empty or invalid declaration item")
		}
	}
	return parts, nil
}

// normalizeSQLTokens gives storage compatibility a formatting-independent
// representation while retaining every identifier, literal, operator and
// constraint token. The admitted DDL does not permit quoted identifiers, but
// quoted CHECK literals are preserved exactly.
func normalizeSQLTokens(source string) string {
	runes := []rune(source)
	var tokens []string
	for index := 0; index < len(runes); {
		char := runes[index]
		if unicode.IsSpace(char) {
			index++
			continue
		}
		if unicode.IsLetter(char) || unicode.IsDigit(char) || char == '_' {
			start := index
			for index < len(runes) && (unicode.IsLetter(runes[index]) || unicode.IsDigit(runes[index]) || runes[index] == '_') {
				index++
			}
			tokens = append(tokens, strings.ToLower(string(runes[start:index])))
			continue
		}
		if char == '\'' || char == '"' {
			quote := char
			start := index
			index++
			for index < len(runes) {
				if runes[index] == quote {
					index++
					if index < len(runes) && runes[index] == quote {
						index++
						continue
					}
					break
				}
				index++
			}
			tokens = append(tokens, string(runes[start:index]))
			continue
		}
		if index+1 < len(runes) {
			pair := string(runes[index : index+2])
			if pair == ">=" || pair == "<=" || pair == "<>" || pair == "!=" || pair == "||" {
				tokens = append(tokens, pair)
				index += 2
				continue
			}
		}
		tokens = append(tokens, string(char))
		index++
	}
	return strings.Join(tokens, "\x1f")
}
