// Package readability enforces five mechanical Go readability rules on ASTs
// using only the Go standard library:
//
//  1. Nesting ("nesting"): inside a function body, no statement may sit more
//     than 2 block levels deep (one if/for/range/switch/select/block, plus one
//     more level inside that). Chained else-if branches stay at the same depth
//     as the initial if. Function literals continue from the enclosing depth
//     instead of resetting to zero. Exception: when the first statement of a
//     for or range loop body is a single guard `if ... { continue | break | return }`
//     (with no else branch), that guard does not add a nesting level.
//
//  2. Inline nesting of objects ("inline-nesting"): a struct or map composite
//     literal sitting directly—or through slice/array literals, address-of (&),
//     or parentheses—inside another struct or map composite literal in the same
//     expression is reported with both outer and inner positions, as is a
//     slice/array literal nested inside another slice/array literal (such as
//     [][]int{{1}}). Struct or map literals inside a standalone slice/array
//     literal (such as []Site{{Tag: "x"}}), plain slice literals ([]string{"a"}),
//     and flat struct literals (Point{X: 1}) are allowed.
//
//  3. Function length ("func-length"): any function, method, or function
//     literal whose body spans more than 40 lines (inclusive from opening '{'
//     to closing '}') is reported.
//
//  4. Conditions ("condition"): any boolean expression in an if condition,
//     for condition, tagless switch case, or return statement with 3 or more
//     && / || operands not wrapped in a named function or variable is reported.
//
//  5. Boolean parameters ("bool-param"): a function, method or function literal
//     with a parameter declared as bool is reported.
//
// For incremental adoption, the package provides a per-file finding-count
// baseline and ratchet: LoadBaseline, WriteBaseline, Ratchet (reports files
// whose count grew or new files with findings), and Shrinkable (reports files
// whose counts shrank as "baseline can shrink to N").
package readability
