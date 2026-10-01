package render

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"text/template"
	"text/template/parse"
)

// wildcard is the path segment for any key or index. A map key equal to it can
// only add a spurious dependency, never hide one.
const wildcard = "\x00*"

const maxValueChoices = 16

// selfRef is a document path that a template reads under the self key, with
// the self key stripped; an empty path is the whole document. An exact
// reference reads only the value at path (for example an if/with/range
// selector or len); otherwise everything below path is read too.
type selfRef struct {
	path  []string
	exact bool
}

// value is what a template expression evaluates to as far as the document is
// concerned: a document path (self), the root data (root), or neither. A
// constant string or integer also carries its text as an index key.
type value struct {
	path    []string
	choices []value
	self    bool
	root    bool
	unknown bool
	key     string
	literal bool
}

type scope map[string]value

type refWalker struct {
	selfKey string
	funcs   template.FuncMap
	mutable map[*parse.VariableNode]bool
	refs    []selfRef
}

// selfRefs returns the document paths that t reads under the self key.
//
// It evaluates the template abstractly, tracking which document path each
// dot, variable and expression refers to. Values that are printed or passed to
// a function are read with everything below them; if/with/range selectors and
// len read only the value itself, and index with constant keys selects a path.
// Named templates and assignments read their whole argument.
func selfRefs(t *template.Template, cfg *config) []selfRef {
	if t.Tree == nil {
		return nil
	}
	mutable := findMutableVariables(t.Root, cfg.funcs)
	w := &refWalker{selfKey: cfg.selfKey, funcs: cfg.funcs, mutable: mutable}
	root := value{root: true}
	w.walk(t.Root, root, scope{"$": root})
	return w.refs
}

type bindingID uint64

type bindingScope map[string]bindingID

type bindingAnalyzer struct {
	next     bindingID
	funcs    template.FuncMap
	mutable  map[bindingID]bool
	parents  map[bindingID][]bindingID
	children map[bindingID][]bindingID
	uses     map[*parse.VariableNode]bindingID
}

func findMutableVariables(root parse.Node, funcs template.FuncMap) map[*parse.VariableNode]bool {
	analyzer := &bindingAnalyzer{next: 1, funcs: funcs, mutable: map[bindingID]bool{}, parents: map[bindingID][]bindingID{}, children: map[bindingID][]bindingID{}, uses: map[*parse.VariableNode]bindingID{}}
	analyzer.walk(root, bindingScope{"$": 1})
	mutable := map[*parse.VariableNode]bool{}
	for variable, binding := range analyzer.uses {
		if analyzer.mutable[binding] {
			mutable[variable] = true
		}
	}
	return mutable
}

func (a *bindingAnalyzer) walk(n parse.Node, scope bindingScope) {
	switch n := n.(type) {
	case *parse.ListNode:
		if n != nil {
			for _, child := range n.Nodes {
				a.walk(child, scope)
			}
		}
	case *parse.ActionNode:
		a.walkPipe(n.Pipe, scope)
	case *parse.PipeNode:
		a.walkPipe(n, scope)
	case *parse.CommandNode:
		a.walkCommand(n, scope)
	case *parse.VariableNode:
		a.uses[n] = scope[n.Ident[0]]
	case *parse.ChainNode:
		a.walk(n.Node, scope)
	case *parse.IfNode:
		a.walkBranch(&n.BranchNode, scope)
	case *parse.RangeNode:
		a.walkBranch(&n.BranchNode, scope)
	case *parse.WithNode:
		a.walkBranch(&n.BranchNode, scope)
	case *parse.BranchNode:
		a.walkBranch(n, scope)
	case *parse.TemplateNode:
		a.walkPipe(n.Pipe, maps.Clone(scope))
	}
}

func (a *bindingAnalyzer) walkBranch(branch *parse.BranchNode, scope bindingScope) {
	branchScope := maps.Clone(scope)
	a.walkPipe(branch.Pipe, branchScope)
	if branch.NodeType == parse.NodeIf {
		if truth, known := literalTruthPipe(branch.Pipe); known {
			if truth {
				a.walk(branch.List, maps.Clone(branchScope))
			} else {
				a.walk(branch.ElseList, maps.Clone(branchScope))
			}
			return
		}
	}
	a.walk(branch.List, maps.Clone(branchScope))
	a.walk(branch.ElseList, maps.Clone(branchScope))
}

func (a *bindingAnalyzer) walkPipe(pipe *parse.PipeNode, scope bindingScope) {
	if pipe == nil {
		return
	}
	for _, command := range pipe.Cmds {
		a.walkCommand(command, scope)
	}
	for _, declaration := range pipe.Decl {
		name := declaration.Ident[0]
		if pipe.IsAssign {
			binding := scope[name]
			a.markMutable(binding)
			a.uses[declaration] = binding
			continue
		}
		a.next++
		scope[name] = a.next
		a.uses[declaration] = a.next
	}
}

func (a *bindingAnalyzer) walkCommand(command *parse.CommandNode, scope bindingScope) {
	name := commandName(command)
	_, overridden := a.funcs[name]
	shortCircuit := !overridden && (name == "and" || name == "or")
	args := command.Args
	if shortCircuit {
		args = args[1:]
	}
	for i, argument := range args {
		if shortCircuit && i > 0 {
			truth, known := literalTruth(args[i-1])
			if known && (name == "or" && truth || name == "and" && !truth) {
				break
			}
		}
		conditional := shortCircuit && i > 0
		argumentScope := scope
		if conditional {
			argumentScope = maps.Clone(scope)
		}
		a.walk(argument, argumentScope)
		if conditional {
			for name, binding := range argumentScope {
				current, exists := scope[name]
				if !exists {
					scope[name] = binding
				} else if current != binding {
					a.next++
					scope[name] = a.next
					a.parents[a.next] = []bindingID{current, binding}
					a.children[current] = append(a.children[current], a.next)
					a.children[binding] = append(a.children[binding], a.next)
					if a.mutable[current] || a.mutable[binding] {
						a.markMutable(a.next)
					}
				}
			}
		}
	}
}

func (a *bindingAnalyzer) markMutable(binding bindingID) {
	if a.mutable[binding] {
		return
	}
	a.mutable[binding] = true
	for _, parent := range a.parents[binding] {
		a.markMutable(parent)
	}
	for _, child := range a.children[binding] {
		a.markMutable(child)
	}
}

func (w *refWalker) add(path []string, exact bool) {
	w.refs = append(w.refs, selfRef{path: path, exact: exact})
}

// read records reading v and everything below it.
func (w *refWalker) read(v value) {
	if len(v.choices) > 0 {
		for _, choice := range v.choices {
			w.read(choice)
		}
		return
	}
	switch {
	case v.self:
		w.add(v.path, false)
	case v.root || v.unknown:
		w.add(nil, false)
	}
}

func (w *refWalker) readExact(v value) {
	if len(v.choices) > 0 {
		for _, choice := range v.choices {
			w.readExact(choice)
		}
		return
	}
	if v.self {
		w.add(v.path, true)
	} else if v.unknown {
		w.read(v)
	}
}

// field returns the value at keys below v.
func (w *refWalker) field(v value, keys ...string) value {
	switch {
	case len(keys) == 0:
		return v
	case len(v.choices) > 0:
		var out value
		hasOut := false
		for _, choice := range v.choices {
			out = mergeValues(out, w.field(choice, keys...), hasOut)
			hasOut = true
		}
		return out
	case v.unknown:
		return value{unknown: true}
	case v.self:
		return value{path: join(v.path, keys...), self: true}
	case v.root && (keys[0] == w.selfKey || keys[0] == wildcard):
		return value{path: keys[1:], self: true}
	}
	return value{}
}

func (w *refWalker) walk(n parse.Node, d value, vars scope) {
	switch n := n.(type) {
	case *parse.ListNode:
		if n == nil {
			return
		}
		for _, c := range n.Nodes {
			w.walk(c, d, vars)
		}
	case *parse.ActionNode:
		v := w.pipe(n.Pipe, d, vars)
		if len(n.Pipe.Decl) == 0 {
			w.read(v)
		}
	case *parse.TemplateNode:
		if n.Pipe != nil {
			w.read(w.pipe(n.Pipe, d, vars))
		}
	case *parse.IfNode:
		w.branch(&n.BranchNode, d, vars)
	case *parse.RangeNode:
		w.branch(&n.BranchNode, d, vars)
	case *parse.WithNode:
		w.branch(&n.BranchNode, d, vars)
	}
}

// branch walks an if, with or range. Its body and else branch each get their
// own scope, and dot is unchanged in an if and in every else branch.
func (w *refWalker) branch(br *parse.BranchNode, d value, vars scope) {
	outer := maps.Clone(vars)
	truthOnly := br.NodeType == parse.NodeIf && !br.Pipe.IsAssign
	for _, decl := range br.Pipe.Decl {
		if variableUsed(br.List, decl.Ident[0]) || variableUsed(br.ElseList, decl.Ident[0]) {
			truthOnly = false
			break
		}
	}
	branchVars := maps.Clone(vars)
	v := w.pipeMode(br.Pipe, d, branchVars, truthOnly)
	w.readExact(v)
	if br.NodeType == parse.NodeIf {
		if truth, known := literalTruthPipe(br.Pipe); known {
			bodyVars := maps.Clone(branchVars)
			elseVars := maps.Clone(branchVars)
			if truth {
				w.walk(br.List, d, bodyVars)
			} else {
				w.walk(br.ElseList, d, elseVars)
			}
			w.mergeBranchVars(vars, outer, bodyVars, elseVars, br.Pipe, br.List, br.ElseList)
			return
		}
	}
	body := d
	bodyVars := maps.Clone(branchVars)
	elseVars := maps.Clone(branchVars)
	switch br.NodeType {
	case parse.NodeWith:
		body = v
	case parse.NodeRange:
		body = w.field(v, wildcard)
		for i, decl := range br.Pipe.Decl {
			// The first of two range variables is the key or index.
			if len(br.Pipe.Decl) == 2 && i == 0 {
				bodyVars[decl.Ident[0]] = value{}
			} else {
				bodyVars[decl.Ident[0]] = body
			}
		}
	default:
	}
	w.walk(br.List, body, bodyVars)
	w.walk(br.ElseList, d, elseVars)
	w.mergeBranchVars(vars, outer, bodyVars, elseVars, br.Pipe, br.List, br.ElseList)
}

func (w *refWalker) mergeBranchVars(vars, outer, body, otherwise scope, pipe *parse.PipeNode, bodyList, elseList *parse.ListNode) {
	local := map[string]bool{}
	if !pipe.IsAssign {
		for _, decl := range pipe.Decl {
			local[decl.Ident[0]] = true
		}
	}
	for _, name := range slices.Sorted(maps.Keys(outer)) {
		if local[name] {
			continue
		}
		bodyValue := body[name]
		elseValue := otherwise[name]
		if declaresInList(bodyList, name) {
			bodyValue = outer[name]
		}
		if declaresInList(elseList, name) {
			elseValue = outer[name]
		}
		merged := mergeValues(bodyValue, elseValue, true)
		if merged.unknown {
			w.read(bodyValue)
			w.read(elseValue)
		}
		vars[name] = merged
	}
}

func declaresInList(list *parse.ListNode, name string) bool {
	if list == nil {
		return false
	}
	for _, node := range list.Nodes {
		action, ok := node.(*parse.ActionNode)
		if !ok || action.Pipe == nil || action.Pipe.IsAssign {
			continue
		}
		for _, decl := range action.Pipe.Decl {
			if decl.Ident[0] == name {
				return true
			}
		}
	}
	return false
}

func variableUsed(n parse.Node, name string) bool {
	return variableUsedIn(n, name, false)
}

func variableUsedIn(n parse.Node, name string, shadowed bool) bool {
	switch n := n.(type) {
	case *parse.ListNode:
		if n != nil {
			for _, child := range n.Nodes {
				if variableUsedIn(child, name, shadowed) {
					return true
				}
			}
		}
	case *parse.ActionNode:
		return variableUsedIn(n.Pipe, name, shadowed)
	case *parse.VariableNode:
		return !shadowed && n.Ident[0] == name
	case *parse.ChainNode:
		return variableUsedIn(n.Node, name, shadowed)
	case *parse.PipeNode:
		if n != nil {
			for _, cmd := range n.Cmds {
				if variableUsedIn(cmd, name, shadowed) {
					return true
				}
			}
		}
	case *parse.CommandNode:
		for _, arg := range n.Args {
			if variableUsedIn(arg, name, shadowed) {
				return true
			}
		}
	case *parse.IfNode:
		return variableUsedIn(&n.BranchNode, name, shadowed)
	case *parse.RangeNode:
		return variableUsedIn(&n.BranchNode, name, shadowed)
	case *parse.WithNode:
		return variableUsedIn(&n.BranchNode, name, shadowed)
	case *parse.BranchNode:
		if variableUsedIn(n.Pipe, name, shadowed) {
			return true
		}
		bodyShadowed := shadowed
		for _, decl := range n.Pipe.Decl {
			if !n.Pipe.IsAssign && decl.Ident[0] == name {
				bodyShadowed = true
			}
		}
		return variableUsedIn(n.List, name, bodyShadowed) || variableUsedIn(n.ElseList, name, bodyShadowed)
	case *parse.TemplateNode:
		return variableUsedIn(n.Pipe, name, shadowed)
	}
	return false
}

func (w *refWalker) pipe(p *parse.PipeNode, d value, vars scope) value {
	return w.pipeMode(p, d, vars, false)
}

func (w *refWalker) pipeMode(p *parse.PipeNode, d value, vars scope, truthOnly bool) value {
	truthModes := make([]bool, len(p.Cmds))
	if truthOnly && len(p.Cmds) > 0 {
		truthModes[len(truthModes)-1] = true
	}
	for i := len(truthModes) - 2; i >= 0; i-- {
		name := commandName(p.Cmds[i+1])
		_, overridden := w.funcs[name]
		if overridden {
			break
		}
		if name == "not" || truthModes[i+1] && (name == "and" || name == "or") {
			truthModes[i] = true
			continue
		}
		break
	}
	var v value
	for i, cmd := range p.Cmds {
		v = w.command(cmd, d, vars, v, i > 0, truthModes[i])
	}
	for _, decl := range p.Decl {
		if p.IsAssign {
			// The assigned variable can outlive this scope.
			w.read(v)
		}
		vars[decl.Ident[0]] = v
	}
	return v
}

func commandName(cmd *parse.CommandNode) string {
	if id, ok := cmd.Args[0].(*parse.IdentifierNode); ok {
		return id.Ident
	}
	return ""
}

// command evaluates cmd; piped is the previous command's result, passed as the
// final argument when hasPiped.
func (w *refWalker) command(cmd *parse.CommandNode, d value, vars scope, piped value, hasPiped, truthOnly bool) value {
	name := ""
	args := cmd.Args
	if args[0] != nil {
		if fn, ok := args[0].(*parse.IdentifierNode); ok {
			name, args = fn.Ident, args[1:]
		}
	}
	_, overridden := w.funcs[name]
	argCount := len(args)
	if hasPiped {
		argCount++
	}
	argTruthOnly := !overridden && (name == "not" && argCount == 1 ||
		truthOnly && (name == "and" || name == "or" || name == "" && len(args) == 1 && !hasPiped))
	shortCircuit := !overridden && (name == "and" || name == "or")
	vals := w.args(name, args, d, vars, argTruthOnly, shortCircuit)
	if hasPiped {
		vals = append(vals, piped)
	}
	switch {
	case name == "" && len(vals) == 1:
		return vals[0]
	case name == "index" && !overridden && len(vals) > 0:
		return w.index(vals[0], vals[1:])
	case name == "len" && !overridden && len(vals) == 1:
		w.readExact(vals[0])
		return value{}
	case name == "not" && !overridden && len(vals) == 1:
		w.readExact(vals[0])
		return value{}
	case truthOnly && (name == "and" || name == "or") && !overridden:
		for _, v := range vals {
			w.readExact(v)
		}
		return value{}
	}
	for _, v := range vals {
		w.read(v)
	}
	return value{}
}

// args evaluates a command's arguments. and/or arguments after a literal that
// decides the result are skipped, and the rest are evaluated in their own scope.
func (w *refWalker) args(name string, args []parse.Node, d value, vars scope, truthOnly, shortCircuit bool) []value {
	vals := make([]value, 0, len(args)+1)
	for i, a := range args {
		conditional := shortCircuit && i > 0
		if conditional {
			truth, known := literalTruth(args[i-1])
			if known && (name == "or" && truth || name == "and" && !truth) {
				break
			}
		}
		argVars := vars
		if conditional {
			argVars = maps.Clone(vars)
		}
		vals = append(vals, w.evalMode(a, d, argVars, truthOnly))
		if conditional {
			w.mergeConditionalAssignments(vars, argVars, a)
		}
	}
	return vals
}

// index returns the value index selects from v with keys.
func (w *refWalker) index(v value, keys []value) value {
	for _, k := range keys {
		choices := k.choices
		if len(choices) == 0 {
			choices = []value{k}
		}
		var selected value
		selectedSet := false
		for _, keyValue := range choices {
			key := wildcard
			if keyValue.literal {
				key = keyValue.key
			}
			if !keyValue.unknown {
				w.read(keyValue)
			}
			candidate := w.field(v, key)
			merged := mergeValues(selected, candidate, selectedSet)
			if merged.unknown {
				w.read(selected)
				w.read(candidate)
			}
			selected = merged
			selectedSet = true
		}
		v = selected
	}
	return v
}

func (w *refWalker) mergeConditionalAssignments(vars, possible scope, expr parse.Node) {
	declared := declaredVariables(expr)
	for _, name := range slices.Sorted(maps.Keys(possible)) {
		candidate := possible[name]
		current, exists := vars[name]
		if exists && equalValue(current, candidate) {
			continue
		}
		if declared[name] {
			if exists {
				merged := mergeValues(current, candidate, true)
				if merged.unknown {
					w.read(current)
					w.read(candidate)
				}
				vars[name] = merged
			} else {
				vars[name] = candidate
			}
			continue
		}
		w.read(candidate)
		if exists {
			w.read(current)
			vars[name] = mergeValues(current, candidate, true)
		} else {
			vars[name] = candidate
		}
	}
}

func declaredVariables(n parse.Node) map[string]bool {
	declared := map[string]bool{}
	var walk func(parse.Node)
	walk = func(n parse.Node) {
		switch n := n.(type) {
		case *parse.ListNode:
			if n != nil {
				for _, child := range n.Nodes {
					walk(child)
				}
			}
		case *parse.ActionNode:
			walk(n.Pipe)
		case *parse.PipeNode:
			if n != nil {
				if !n.IsAssign {
					for _, decl := range n.Decl {
						declared[decl.Ident[0]] = true
					}
				}
				for _, cmd := range n.Cmds {
					walk(cmd)
				}
			}
		case *parse.CommandNode:
			for _, arg := range n.Args {
				walk(arg)
			}
		case *parse.ChainNode:
			walk(n.Node)
		}
	}
	walk(n)
	return declared
}

func literalTruth(n parse.Node) (bool, bool) {
	switch n := n.(type) {
	case *parse.BoolNode:
		return n.True, true
	case *parse.StringNode:
		return n.Text != "", true
	case *parse.NumberNode:
		switch {
		case n.IsInt:
			return n.Int64 != 0, true
		case n.IsUint:
			return n.Uint64 != 0, true
		case n.IsFloat:
			return n.Float64 != 0, true
		case n.IsComplex:
			return n.Complex128 != 0, true
		}
	case *parse.NilNode:
		return false, true
	case *parse.PipeNode:
		if n != nil && len(n.Cmds) == 1 {
			return literalTruth(n.Cmds[0])
		}
	case *parse.CommandNode:
		if len(n.Args) == 1 {
			return literalTruth(n.Args[0])
		}
	}
	return false, false
}

func literalTruthPipe(pipe *parse.PipeNode) (bool, bool) {
	if pipe == nil || len(pipe.Cmds) != 1 {
		return false, false
	}
	return literalTruth(pipe.Cmds[0])
}

func equalValue(a, b value) bool {
	return a.self == b.self && a.root == b.root && a.unknown == b.unknown &&
		a.key == b.key && a.literal == b.literal && slices.Equal(a.path, b.path) && equalValues(a.choices, b.choices)
}

func mergeValues(a, b value, hasA bool) value {
	if !hasA {
		return b
	}
	if equalValue(a, b) {
		return a
	}
	if a.unknown || b.unknown {
		return value{unknown: true}
	}
	choices := make([]value, 0, len(a.choices)+len(b.choices)+2)
	appendChoices := func(v value) {
		if len(v.choices) == 0 {
			choices = append(choices, v)
			return
		}
		choices = append(choices, v.choices...)
	}
	appendChoices(a)
	appendChoices(b)
	unique := choices[:0]
	for _, choice := range choices {
		found := false
		for _, existing := range unique {
			if equalValue(choice, existing) {
				found = true
				break
			}
		}
		if !found {
			unique = append(unique, choice)
		}
	}
	if len(unique) > maxValueChoices {
		return value{unknown: true}
	}
	return value{choices: unique}
}

func dynamicValue(v value) value {
	v.literal = false
	v.key = ""
	v.choices = slices.Clone(v.choices)
	for i := range v.choices {
		v.choices[i] = dynamicValue(v.choices[i])
	}
	return v
}

func equalValues(a, b []value) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !equalValue(a[i], b[i]) {
			return false
		}
	}
	return true
}

func (w *refWalker) eval(n parse.Node, d value, vars scope) value {
	return w.evalMode(n, d, vars, false)
}

func (w *refWalker) evalMode(n parse.Node, d value, vars scope, truthOnly bool) value {
	switch n := n.(type) {
	case *parse.DotNode:
		return d
	case *parse.FieldNode:
		return w.field(d, n.Ident...)
	case *parse.VariableNode:
		v := w.field(vars[n.Ident[0]], n.Ident[1:]...)
		if w.mutable[n] {
			v = dynamicValue(v)
		}
		return v
	case *parse.ChainNode:
		return w.field(w.eval(n.Node, d, vars), n.Field...)
	case *parse.PipeNode:
		return w.pipeMode(n, d, vars, truthOnly && len(n.Decl) == 0)
	case *parse.StringNode:
		return value{key: n.Text, literal: true}
	case *parse.NumberNode:
		if n.IsInt {
			return value{key: strconv.FormatInt(n.Int64, 10), literal: true}
		}
	}
	return value{}
}

// join returns path extended by keys without sharing path's backing array.
func join(path []string, keys ...string) []string {
	return append(slices.Clip(path), keys...)
}

// pathTree indexes leaves by path segment.
type pathTree struct {
	leaf     *leaf
	children map[string]*pathTree
	keys     []string
	group    *graphNode
	refs     map[string]*graphNode
}

type graphNode struct {
	leaf *leaf
	deps []*graphNode
}

func (t *pathTree) insert(l *leaf) {
	for _, key := range l.segs {
		if t.children == nil {
			t.children = map[string]*pathTree{}
		}
		if t.children[key] == nil {
			t.children[key] = &pathTree{}
		}
		t = t.children[key]
	}
	t.leaf = l
}

// match calls fn for every graph node a reference to path depends on.
func (t *pathTree) match(path []string, exact bool, fn func(*graphNode)) {
	if len(path) == 0 {
		if exact {
			if t.leaf != nil {
				fn(t.leaf.node)
			}
		} else {
			fn(t.group)
		}
		return
	}
	if t.leaf != nil {
		fn(t.leaf.node)
	}
	if path[0] != wildcard {
		if c := t.children[path[0]]; c != nil {
			c.match(path[1:], exact, fn)
		}
		return
	}
	for _, key := range t.keys {
		t.children[key].match(path[1:], exact, fn)
	}
}

func (t *pathTree) reference(ref selfRef) *graphNode {
	key := fmt.Sprintf("%q:%t", ref.path, ref.exact)
	if node := t.refs[key]; node != nil {
		return node
	}
	node := &graphNode{}
	seen := map[*graphNode]bool{}
	t.match(ref.path, ref.exact, func(dep *graphNode) {
		if !seen[dep] {
			seen[dep] = true
			node.deps = append(node.deps, dep)
		}
	})
	if t.refs == nil {
		t.refs = map[string]*graphNode{}
	}
	t.refs[key] = node
	return node
}

func (t *pathTree) buildGroup() *graphNode {
	t.group = &graphNode{}
	if t.leaf != nil {
		t.group.deps = append(t.group.deps, t.leaf.node)
	}
	t.keys = slices.Sorted(maps.Keys(t.children))
	for _, key := range t.keys {
		t.group.deps = append(t.group.deps, t.children[key].buildGroup())
	}
	return t.group
}

// buildDeps resolves each leaf's self references to the leaves it depends on.
func buildDeps(leaves []*leaf) {
	tree := &pathTree{}
	for _, l := range leaves {
		l.node = &graphNode{leaf: l}
		tree.insert(l)
	}
	tree.buildGroup()
	for _, x := range leaves {
		seen := map[*graphNode]bool{}
		for _, ref := range x.refs {
			y := tree.reference(ref)
			if !seen[y] {
				seen[y] = true
				x.node.deps = append(x.node.deps, y)
			}
		}
	}
}

// topoSort returns the leaves ordered so that every leaf appears after its
// dependencies. It returns a *CycleError (which is ErrReferenceCycle) if the
// dependency graph contains a cycle.
func topoSort(leaves []*leaf) ([]*leaf, error) {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[*graphNode]int, len(leaves))
	var order []*leaf
	var stack []*graphNode

	var visit func(l *graphNode) error
	visit = func(l *graphNode) error {
		switch color[l] {
		case black:
			return nil
		case gray:
			idx := 0
			for i, s := range stack {
				if s == l {
					idx = i
					break
				}
			}
			cycle := make([]string, 0, len(stack)-idx+1)
			for _, s := range stack[idx:] {
				if s.leaf != nil {
					cycle = append(cycle, s.leaf.path)
				}
			}
			if l.leaf != nil {
				cycle = append(cycle, l.leaf.path)
			} else if len(cycle) > 0 {
				cycle = append(cycle, cycle[0])
			}
			return &CycleError{Fields: cycle}
		}
		color[l] = gray
		stack = append(stack, l)
		for _, d := range l.deps {
			if err := visit(d); err != nil {
				return err
			}
		}
		stack = stack[:len(stack)-1]
		color[l] = black
		if l.leaf != nil {
			order = append(order, l.leaf)
		}
		return nil
	}

	for _, l := range leaves {
		if err := visit(l.node); err != nil {
			return nil, err
		}
	}
	return order, nil
}
