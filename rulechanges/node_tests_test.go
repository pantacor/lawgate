package rulechanges

import (
	"slices"
	"strings"
	"testing"
)

func aliasedNodeSources() map[string]string {
	return map[string]string{
		"lawgate.json":           `{"modules":[{"path":"web","language":"node","laws":"web/LAWS.md","machines":false}]}`,
		"web/LAWS.md":            "### web-law\n- Enforced by: test `ordinary node test`\n",
		"web/law.test.cjs":       "const check = require('node:test')\nconst helper = require('./helper')\ncheck('ordinary node test', () => helper())\n",
		"web/helper.js":          "module.exports = () => {}\n",
		"web/test/other.js":      "module.exports = {}\n",
		"web/src/app.js":         "module.exports = {}\n",
		"web/node_modules/x.cjs": "module.exports = {}\n",
	}
}

// TestAliasedNodeEnforcerFailsClosed: a named Node enforcer registered
// through an alias of test binds to no file the scan can find, so every
// test file of the module is protected with its imports, and the enforcer
// is reported.
func TestAliasedNodeEnforcerFailsClosed(t *testing.T) {
	result := classify(mapTree(aliasedNodeSources()))
	err := result.err()
	if err == nil || !strings.Contains(err.Error(), "ordinary node test") {
		t.Fatalf("unbound Node enforcer was not reported: %v", err)
	}
	actual := sortedPaths(result.rules)
	for _, name := range []string{"web/law.test.cjs", "web/helper.js", "web/test/other.js"} {
		if !slices.Contains(actual, name) {
			t.Errorf("%s is not a rule file; rule files = %v", name, actual)
		}
	}
	for _, name := range []string{"web/src/app.js", "web/node_modules/x.cjs"} {
		if slices.Contains(actual, name) {
			t.Errorf("%s became a rule file; rule files = %v", name, actual)
		}
	}
}

// TestDeclaredNodeEnforcerIsBound: the same enforcer declared with test
// binds to its file and nothing is reported.
func TestDeclaredNodeEnforcerIsBound(t *testing.T) {
	files := aliasedNodeSources()
	files["web/law.test.cjs"] = "const test = require('node:test')\nconst helper = require('./helper')\ntest('ordinary node test', () => helper())\n"
	requireRuleFiles(t, files, []string{"web/law.test.cjs", "web/helper.js"}, []string{"web/test/other.js"})
}

// requireUnbound fails unless the module's enforcer is reported unbound
// and its real (aliased) test file is protected.
func requireUnbound(t *testing.T, files map[string]string) {
	t.Helper()
	result := classify(mapTree(files))
	err := result.err()
	if err == nil || !strings.Contains(err.Error(), "ordinary node test") {
		t.Fatalf("unbound Node enforcer was not reported: %v", err)
	}
	if !result.rules["web/law.test.cjs"] {
		t.Errorf("web/law.test.cjs is not a rule file; rule files = %v", sortedPaths(result.rules))
	}
}

// TestNodeEnforcerBindsOnlyToTestCalls: a method named test, a comment and
// a string that spell the call do not declare the enforcer, and neither
// does a file Node's runner never loads, so the aliased enforcer still
// fails closed.
func TestNodeEnforcerBindsOnlyToTestCalls(t *testing.T) {
	impostors := map[string]string{
		"web/src/app.js":     "const ok = /x/.test('ordinary node test')\n",
		"web/src/main.js":    "test('ordinary node test', () => {})\n",
		"web/test/method.js": "const ok = pattern . test('ordinary node test')\n",
		"web/test/note.js":   "// test('ordinary node test')\n/* it('ordinary node test') */\n",
		"web/test/text.js":   "const s = \"test('ordinary node test')\"\nconst u = `it('ordinary node test')`\n",
	}
	for name, source := range impostors {
		files := aliasedNodeSources()
		files[name] = source
		t.Run(name, func(t *testing.T) { requireUnbound(t, files) })
	}
}

// TestConfiguredNodeTestFileBindsTheEnforcer: a test file the module's
// test_command names is loaded whatever its name, so its test(...) call
// binds the enforcer.
func TestConfiguredNodeTestFileBindsTheEnforcer(t *testing.T) {
	files := aliasedNodeSources()
	files["lawgate.json"] = `{"modules":[{"path":"web","language":"node","laws":"web/LAWS.md","machines":false,"test_command":["node","--test","checks/law.spec.cjs"]}]}`
	files["web/checks/law.spec.cjs"] = "const { test } = require('node:test')\ntest('ordinary node test', () => {})\n"
	requireRuleFiles(t, files, []string{"web/checks/law.spec.cjs"}, []string{"web/test/other.js"})
}

// TestNodeJSXSourcesAreFollowed: a .tsx or .jsx file a law test loads is
// followed through its own imports.
func TestNodeJSXSourcesAreFollowed(t *testing.T) {
	files := aliasedNodeSources()
	files["web/law.test.cjs"] = "const test = require('node:test')\n"
	files["web/law.test.ts"] = "import { test } from 'node:test'\nimport { a } from './a'\nimport { c } from './c.jsx'\ntest('ordinary node test', () => a(c))\n"
	files["web/a.tsx"] = "import { b } from './b'\nexport const a = b\n"
	files["web/b.ts"] = "export const b = () => {}\n"
	files["web/c.jsx"] = "import { d } from './d.js'\nexport const c = d\n"
	files["web/d.js"] = "export const d = 1\n"
	requireRuleFiles(t, files, []string{"web/a.tsx", "web/b.ts", "web/c.jsx", "web/d.js"}, nil)
}

// TestNodeDeclaredTestsReadsCallsOnly: real calls bind, including one in a
// template substitution and one after a regular expression holding a
// quote; spellings inside literals and member calls do not.
func TestNodeDeclaredTestsReadsCallsOnly(t *testing.T) {
	names := map[string]bool{"a": true, "b": true, "c": true, "d": true, "e": true}
	source := "const r = /'/\ntest('a', () => {})\n" +
		"const s = `${it(\"b\", () => {})} test('x')`\n" +
		"it.skip(`c`, () => {})\n" +
		"t.test('d')\nconst u = 'test(\"e\")'\nreturn /\"/.test('e')\n"
	declared := nodeDeclaredTests(names, []byte(source))
	slices.Sort(declared)
	if !slices.Equal(declared, []string{"a", "b", "c"}) {
		t.Fatalf("declared = %v; want [a b c]", declared)
	}
}
