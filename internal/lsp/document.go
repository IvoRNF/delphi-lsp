package lsp

import (
	"bufio"
	"regexp"
	"sort"
	"strings"
)

const (
	symbolModule   = 2
	symbolClass    = 5
	symbolMethod   = 6
	symbolProperty = 7
	symbolField    = 8
	symbolFunction = 12
	symbolVariable = 13
	symbolConstant = 14
)

type Symbol struct {
	Name           string
	Detail         string
	Documentation  string
	Owner          string
	Parents        []string
	Kind           int
	Range          Range
	Selection      Range
	Scope          Range
	Implementation bool
}

type Document struct {
	URI, Text   string
	Symbols     []Symbol
	Uses        []UnitReference
	Diagnostics []Diagnostic
}

// UnitReference is a unit name in a Delphi uses clause.
type UnitReference struct {
	Name  string
	Range Range
}

type routineContext struct {
	index int
	body  bool
	depth int
}

var declaration = regexp.MustCompile(`(?i)^\s*(procedure|function|constructor|destructor|type|var|const|property)\s+([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)?)(.*)$`)

// Delphi permits the last field in a record (and the last declaration in a
// var block) to omit its trailing semicolon.
var typedVariable = regexp.MustCompile(`(?i)^\s*([A-Za-z_][A-Za-z0-9_]*(?:\s*,\s*[A-Za-z_][A-Za-z0-9_]*)*)\s*:\s*([^;]+?)(?:\s*;|\s*$)`)
var typeDefinition = regexp.MustCompile(`(?i)^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(class|record|interface|dispinterface|object)\b\s*(?:\(([^)]*)\))?`)
var typeAlias = regexp.MustCompile(`(?i)^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.+);`)
var unitHeader = regexp.MustCompile(`(?i)^\s*unit\s+([A-Za-z_][A-Za-z0-9_.]*)\s*;`)
var usesStart = regexp.MustCompile(`(?i)^\s*uses\b(.*)$`)
var unitName = regexp.MustCompile(`(?i)^\s*([A-Za-z_][A-Za-z0-9_.]*)`)

func Parse(uri, text string) *Document {
	// LSP requires publishDiagnostics.diagnostics to be an array. Keep this
	// non-nil even when parsing finds no diagnostics so JSON encodes it as []
	// rather than null (which some clients, including Neovim, cannot handle).
	document := &Document{URI: uri, Text: text, Diagnostics: []Diagnostic{}}
	lines := strings.Split(text, "\n")
	active := []bool{true}
	inVarSection, routineBody := false, false
	inTypeSection := false
	inImplementation, inUses := false, false
	currentRoutine, currentTypeIndex, routineDepth := -1, -1, 0
	currentType := ""
	routineStack := []routineContext{}
	pendingTypedDeclaration, pendingTypedOwner := "", ""
	pendingTypedStart := 0
	pendingTypedImplementation := false
	endPosition := func(line int) Position {
		if line < 0 {
			line = 0
		}
		return Position{Line: line, Character: len(lines[line])}
	}
	closeRoutine := func(line int) {
		if currentRoutine >= 0 {
			if line < document.Symbols[currentRoutine].Selection.Start.Line {
				line = document.Symbols[currentRoutine].Selection.Start.Line
			}
			document.Symbols[currentRoutine].Scope.End = endPosition(line)
		}
		if len(routineStack) > 0 {
			previous := routineStack[len(routineStack)-1]
			routineStack = routineStack[:len(routineStack)-1]
			currentRoutine, routineBody, routineDepth = previous.index, previous.body, previous.depth
			return
		}
		currentRoutine, routineDepth, routineBody = -1, 0, false
	}
	closeType := func(line int) {
		if currentTypeIndex >= 0 {
			document.Symbols[currentTypeIndex].Scope.End = endPosition(line)
		}
		currentType, currentTypeIndex = "", -1
	}
	consumeTypedVariableLine := func(line string, lineNumber int, owner string, implementationOnly bool) bool {
		if pendingTypedDeclaration != "" {
			pendingTypedDeclaration += "\n" + line
			if addTypedVariables(document, pendingTypedDeclaration, pendingTypedStart, pendingTypedOwner, pendingTypedImplementation) {
				pendingTypedDeclaration = ""
				return true
			}
			if typedVariableContinues(line) {
				return true
			}
			pendingTypedDeclaration = ""
			return false
		}
		if addTypedVariables(document, line, lineNumber, owner, implementationOnly) {
			return true
		}
		if typedVariableContinues(line) {
			pendingTypedDeclaration, pendingTypedOwner = line, owner
			pendingTypedStart, pendingTypedImplementation = lineNumber, implementationOnly
			return true
		}
		return false
	}

	for lineNumber, line := range lines {
		trimmed, upper := strings.TrimSpace(line), strings.ToUpper(strings.TrimSpace(line))
		if strings.HasPrefix(upper, "{$IFDEF ") || strings.HasPrefix(upper, "{$IFNDEF ") {
			active = append(active, active[len(active)-1])
			continue
		}
		if strings.HasPrefix(upper, "{$ELSE") && len(active) > 1 {
			active[len(active)-1] = !active[len(active)-2] && active[len(active)-1]
			continue
		}
		if strings.HasPrefix(upper, "{$ENDIF") && len(active) > 1 {
			active = active[:len(active)-1]
			continue
		}
		if !active[len(active)-1] {
			continue
		}
		if match := unitHeader.FindStringSubmatch(line); match != nil {
			start := strings.Index(strings.ToLower(line), strings.ToLower(match[1]))
			selection := Range{Start: Position{Line: lineNumber, Character: start}, End: Position{Line: lineNumber, Character: start + len(match[1])}}
			document.Symbols = append(document.Symbols, Symbol{Name: match[1], Detail: "unit " + match[1], Kind: symbolModule, Range: Range{Start: Position{Line: lineNumber}, End: endPosition(lineNumber)}, Selection: selection})
			continue
		}
		if strings.EqualFold(trimmed, "implementation") {
			closeRoutine(lineNumber - 1)
			inImplementation, inUses, inTypeSection, inVarSection = true, false, false, false
			continue
		}
		if match := usesStart.FindStringSubmatch(line); match != nil {
			inUses = true
			addUses(document, match[1], lineNumber, len(line)-len(match[1]))
			if strings.Contains(match[1], ";") {
				inUses = false
			}
			continue
		}
		if inUses {
			addUses(document, line, lineNumber, 0)
			if strings.Contains(line, ";") {
				inUses = false
			}
			continue
		}

		if strings.EqualFold(trimmed, "type") {
			inTypeSection = true
			continue
		}
		if currentRoutine < 0 && currentType == "" && strings.EqualFold(trimmed, "var") {
			inVarSection = true
			inTypeSection = false
			continue
		}
		if currentRoutine < 0 && currentType == "" && inTypeSection {
			if match := typeDefinition.FindStringSubmatch(line); match != nil {
				start := strings.Index(strings.ToLower(line), strings.ToLower(match[1]))
				selection := Range{Start: Position{Line: lineNumber, Character: start}, End: Position{Line: lineNumber, Character: start + len(match[1])}}
				document.Symbols = append(document.Symbols, Symbol{Name: match[1], Detail: match[1] + " = " + strings.ToLower(match[2]), Documentation: summaryBefore(lines, lineNumber), Kind: symbolClass, Parents: typeParents(match[3]), Range: Range{Start: Position{Line: lineNumber}, End: endPosition(lineNumber)}, Selection: selection, Scope: Range{Start: selection.Start, End: endPosition(len(lines) - 1)}})
				currentType, currentTypeIndex, inVarSection = match[1], len(document.Symbols)-1, false
				continue
			}
		}
		if currentRoutine < 0 && currentType == "" && inTypeSection {
			if match := typeAlias.FindStringSubmatch(line); match != nil {
				start := strings.Index(strings.ToLower(line), strings.ToLower(match[1]))
				selection := Range{Start: Position{Line: lineNumber, Character: start}, End: Position{Line: lineNumber, Character: start + len(match[1])}}
				document.Symbols = append(document.Symbols, Symbol{Name: match[1], Detail: strings.TrimSpace(line), Documentation: summaryBefore(lines, lineNumber), Kind: symbolClass, Range: Range{Start: Position{Line: lineNumber}, End: endPosition(lineNumber)}, Selection: selection})
				continue
			}
		}
		if currentRoutine < 0 && currentType != "" && strings.HasPrefix(strings.ToLower(trimmed), "end") {
			closeType(lineNumber)
			continue
		}
		if match := declaration.FindStringSubmatch(line); match != nil {
			word, rawName := strings.ToLower(match[1]), match[2]
			if word == "var" && strings.Contains(match[3], ":") {
				owner := ""
				if currentRoutine >= 0 {
					owner = document.Symbols[currentRoutine].Name
				} else if currentType != "" {
					owner = currentType
				}
				// Recombine the first identifier captured by declaration with the
				// remainder so addTypedVariables indexes every comma-separated name.
				addTypedVariables(document, rawName+match[3], lineNumber, owner, inImplementation && owner == "")
				inVarSection = true
				continue
			}
			name := rawName
			if dot := strings.LastIndex(name, "."); dot >= 0 {
				name = name[dot+1:]
			}
			kind := symbolVariable
			switch word {
			case "function", "procedure", "constructor", "destructor":
				kind = symbolFunction
			case "type":
				kind = symbolClass
			case "const":
				kind = symbolConstant
			case "property":
				kind = symbolProperty
			}
			isRoutine := kind == symbolFunction && currentType == ""
			if currentType != "" && kind == symbolFunction {
				kind = symbolMethod
			}
			if isRoutine && currentRoutine >= 0 {
				routineStack = append(routineStack, routineContext{index: currentRoutine, body: routineBody, depth: routineDepth})
			} else if isRoutine {
				closeRoutine(lineNumber - 1)
			}
			owner := ""
			if currentType != "" && (kind == symbolMethod || kind == symbolProperty) {
				owner = currentType
			}
			if strings.Contains(rawName, ".") && kind == symbolFunction {
				owner = strings.TrimSuffix(rawName, "."+name)
			}
			start := strings.Index(strings.ToLower(line), strings.ToLower(rawName))
			if start < 0 {
				start = 0
			}
			nameStart := start + strings.LastIndex(rawName, ".") + 1
			selection := Range{Start: Position{Line: lineNumber, Character: nameStart}, End: Position{Line: lineNumber, Character: nameStart + len(name)}}
			// Declarations in a unit implementation section are unit-private.
			// Members already carry an owner and are not included in ordinary
			// cross-unit name lookup, but standalone routines must be marked too.
			implementationOnly := inImplementation && (kind == symbolFunction || (kind == symbolVariable && owner == ""))
			symbol := Symbol{Name: name, Detail: strings.TrimSpace(match[1] + " " + rawName + match[3]), Documentation: summaryBefore(lines, lineNumber), Owner: owner, Kind: kind, Range: Range{Start: Position{Line: lineNumber}, End: endPosition(lineNumber)}, Selection: selection, Scope: Range{Start: selection.Start, End: endPosition(len(lines) - 1)}, Implementation: implementationOnly}
			document.Symbols = append(document.Symbols, symbol)
			if word == "type" && (strings.Contains(strings.ToLower(match[3]), "= class") || strings.Contains(strings.ToLower(match[3]), "= record")) {
				closeType(lineNumber - 1)
				currentType, currentTypeIndex = name, len(document.Symbols)-1
				inVarSection = false
				continue
			}
			if isRoutine {
				currentRoutine = len(document.Symbols) - 1
				addParameters(document, line, lineNumber, name)
				if word == "function" {
					addFunctionResult(document, match[3], lineNumber, selection, name)
				}
				inVarSection = false
			} else if word == "var" {
				inVarSection = true
			}
			continue
		}

		if currentRoutine >= 0 {
			if strings.EqualFold(trimmed, "var") {
				inVarSection = true
				continue
			}
			if inVarSection && consumeTypedVariableLine(line, lineNumber, document.Symbols[currentRoutine].Name, false) {
				continue
			}
			lower := strings.ToLower(trimmed)
			if strings.Contains(lower, "begin") {
				routineBody = true
				routineDepth += strings.Count(lower, "begin")
				inVarSection = false
			}
			if routineBody && strings.HasPrefix(lower, "end") {
				routineDepth--
				if routineDepth <= 0 {
					closeRoutine(lineNumber)
				}
			}
		} else if currentType != "" {
			if consumeTypedVariableLine(line, lineNumber, currentType, false) {
				continue
			}
		} else if inVarSection {
			if consumeTypedVariableLine(line, lineNumber, "", inImplementation) {
				continue
			}
			if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
				inVarSection = false
			}
		}
		// if strings.Contains(upper, "TODO") {
		// 	document.Diagnostics = append(document.Diagnostics, Diagnostic{Range: Range{Start: Position{Line: lineNumber}, End: endPosition(lineNumber)}, Severity: 3, Source: "delphi-lsp", Message: "TODO"})
		// }
	}
	closeRoutine(len(lines) - 1)
	closeType(len(lines) - 1)
	if len(active) != 1 {
		document.Diagnostics = append(document.Diagnostics, Diagnostic{Severity: 1, Source: "delphi-lsp", Message: "Unclosed compiler directive ({$IFDEF / {$IFNDEF)"})
	}
	sort.SliceStable(document.Symbols, func(i, j int) bool { return document.Symbols[i].Name < document.Symbols[j].Name })
	return document
}

func typeParents(raw string) []string {
	var parents []string
	for _, name := range strings.Split(raw, ",") {
		name = strings.TrimSpace(name)
		if name != "" {
			parents = append(parents, name)
		}
	}
	return parents
}
func addUses(document *Document, text string, lineNumber, offset int) {
	cursor := 0
	for _, part := range strings.Split(text, ",") {
		if semicolon := strings.Index(part, ";"); semicolon >= 0 {
			part = part[:semicolon]
		}
		match := unitName.FindStringSubmatch(part)
		if match != nil {
			start := strings.Index(strings.ToLower(part), strings.ToLower(match[1]))
			character := offset + cursor + start
			document.Uses = append(document.Uses, UnitReference{Name: match[1], Range: Range{Start: Position{Line: lineNumber, Character: character}, End: Position{Line: lineNumber, Character: character + len(match[1])}}})
		}
		cursor += len(part) + 1
	}
}

func addTypedVariables(document *Document, line string, lineNumber int, owner string, implementationOnly bool) bool {
	match := typedVariable.FindStringSubmatch(line)
	if match == nil {
		return false
	}
	kind := symbolVariable
	if owner != "" {
		kind = symbolField
	}
	for _, name := range strings.Split(match[1], ",") {
		name = strings.TrimSpace(name)
		start := strings.Index(strings.ToLower(line), strings.ToLower(name))
		selection := Range{Start: positionAtOffset(line, lineNumber, start), End: positionAtOffset(line, lineNumber, start+len(name))}
		document.Symbols = append(document.Symbols, Symbol{Name: name, Detail: name + ": " + strings.TrimSpace(match[2]), Owner: owner, Kind: kind, Range: Range{Start: Position{Line: lineNumber}, End: positionAtOffset(line, lineNumber, len(line))}, Selection: selection, Implementation: implementationOnly && owner == ""})
	}
	return true
}

// typedVariableContinues reports a declaration name list that continues on
// the next physical line, such as "FirstValue," followed by "SecondValue:
// Integer;". Delphi treats newlines as ordinary whitespace in declarations.
func typedVariableContinues(line string) bool {
	return strings.HasSuffix(strings.TrimSpace(line), ",")
}

func positionAtOffset(text string, firstLine, offset int) Position {
	if offset < 0 {
		offset = 0
	}
	if offset > len(text) {
		offset = len(text)
	}
	prefix := text[:offset]
	line := firstLine + strings.Count(prefix, "\n")
	if lastNewline := strings.LastIndex(prefix, "\n"); lastNewline >= 0 {
		return Position{Line: line, Character: len(prefix) - lastNewline - 1}
	}
	return Position{Line: line, Character: len(prefix)}
}

func addParameters(document *Document, line string, lineNumber int, owner string) {
	open, close := strings.Index(line, "("), strings.LastIndex(line, ")")
	if open < 0 || close <= open {
		return
	}
	for _, group := range strings.Split(line[open+1:close], ";") {
		group = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(group, "const "), "var "), "out "), "constref "))
		parts := strings.SplitN(group, ":", 2)
		if len(parts) != 2 {
			continue
		}
		for _, name := range strings.Split(parts[0], ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			start := strings.Index(strings.ToLower(line), strings.ToLower(name))
			selection := Range{Start: Position{Line: lineNumber, Character: start}, End: Position{Line: lineNumber, Character: start + len(name)}}
			document.Symbols = append(document.Symbols, Symbol{Name: name, Detail: strings.TrimSpace(group), Owner: owner, Kind: symbolVariable, Range: Range{Start: Position{Line: lineNumber}, End: Position{Line: lineNumber, Character: len(line)}}, Selection: selection})
		}
	}
}

// addFunctionResult models Delphi's implicit Result variable. It belongs to
// the function's local scope even though it has no written declaration.
func addFunctionResult(document *Document, suffix string, lineNumber int, selection Range, owner string) {
	returnType := ""
	if close := strings.LastIndex(suffix, ")"); close >= 0 {
		suffix = suffix[close+1:]
	}
	if colon := strings.Index(suffix, ":"); colon >= 0 {
		returnType = strings.TrimSpace(suffix[colon+1:])
		returnType = strings.TrimSpace(strings.TrimSuffix(returnType, ";"))
	}
	detail := "Result"
	if returnType != "" {
		detail += ": " + returnType
	}
	document.Symbols = append(document.Symbols, Symbol{
		Name:      "Result",
		Detail:    detail,
		Owner:     owner,
		Kind:      symbolVariable,
		Range:     Range{Start: selection.Start, End: selection.End},
		Selection: selection,
	})
}

func wordAt(text string, position Position) string {
	lines := strings.Split(text, "\n")
	if position.Line < 0 || position.Line >= len(lines) {
		return ""
	}
	line := lines[position.Line]
	if position.Character > len(line) {
		position.Character = len(line)
	}
	start, end := position.Character, position.Character
	for start > 0 && isWordChar(line[start-1]) {
		start--
	}
	for end < len(line) && isWordChar(line[end]) {
		end++
	}
	return line[start:end]
}

func isWordChar(character byte) bool {
	return character == '_' ||
		character >= '0' && character <= '9' ||
		character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z'
}

// prefixAt returns the partial identifier immediately before the cursor, used
// to scope completion results to what the user has already typed.
func prefixAt(text string, position Position) string {
	lines := strings.Split(text, "\n")
	if position.Line < 0 || position.Line >= len(lines) {
		return ""
	}
	line := lines[position.Line]
	if position.Character > len(line) {
		position.Character = len(line)
	}
	start := position.Character
	for start > 0 && isWordChar(line[start-1]) {
		start--
	}
	return line[start:position.Character]
}

// unitPrefixAt is prefixAt for a Delphi unit name, which may be scoped with
// dots (for example, Company.Data.Sql).
func unitPrefixAt(text string, position Position) string {
	lines := strings.Split(text, "\n")
	if position.Line < 0 || position.Line >= len(lines) {
		return ""
	}
	line := lines[position.Line]
	if position.Character > len(line) {
		position.Character = len(line)
	}
	start := position.Character
	for start > 0 && (isWordChar(line[start-1]) || line[start-1] == '.') {
		start--
	}
	return line[start:position.Character]
}

// usesClauseAt reports whether position is inside a uses declaration. It
// deliberately follows the parser's line-oriented treatment of uses clauses,
// including multi-line declarations.
func usesClauseAt(text string, position Position) bool {
	lines := strings.Split(text, "\n")
	if position.Line < 0 || position.Line >= len(lines) {
		return false
	}
	inUses := false
	for lineNumber, line := range lines[:position.Line+1] {
		if !inUses {
			if usesStart.FindStringSubmatch(line) == nil {
				continue
			}
			inUses = true
		}
		end := len(line)
		if lineNumber == position.Line && position.Character < end {
			end = position.Character
		}
		if strings.Contains(line[:end], ";") {
			inUses = false
			if lineNumber == position.Line {
				return false
			}
			continue
		}
		if lineNumber == position.Line {
			return true
		}
	}
	return false
}

// unitNameOf extracts the unit name from the top of a file without a full
// parse, stopping early so the background indexer can map unit names cheaply.
func unitNameOf(text string) string {
	scanner := bufio.NewScanner(strings.NewReader(text))
	for lines := 0; lines < 60 && scanner.Scan(); lines++ {
		if match := unitHeader.FindStringSubmatch(scanner.Text()); match != nil {
			return match[1]
		}
	}
	return ""
}
