// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package parse

import (
	"fmt"
	"runtime"
	"testing"
	"time"
)

// Make the types prettyprint.
var itemName = map[itemType]string{
	itemError:      "error",
	itemEOF:        "EOF",
	itemNot:        "!",
	itemAnd:        "&&",
	itemOr:         "||",
	itemGreater:    ">",
	itemLess:       "<",
	itemGreaterEq:  ">=",
	itemLessEq:     "<=",
	itemEq:         "==",
	itemNotEq:      "!=",
	itemPlus:       "+",
	itemMinus:      "-",
	itemMult:       "*",
	itemDiv:        "/",
	itemMod:        "%",
	itemNumber:     "number",
	itemComma:      ",",
	itemLeftParen:  "(",
	itemRightParen: ")",
	itemString:     "string",
	itemFunc:       "func",
}

func (i itemType) String() string {
	s := itemName[i]
	if s == "" {
		return fmt.Sprintf("item%d", int(i))
	}
	return s
}

type lexTest struct {
	name  string
	input string
	items []item
}

var (
	tEOF   = item{itemEOF, 0, ""}
	tLt    = item{itemLess, 0, "<"}
	tGt    = item{itemGreater, 0, ">"}
	tOr    = item{itemOr, 0, "||"}
	tNot   = item{itemNot, 0, "!"}
	tAnd   = item{itemAnd, 0, "&&"}
	tLtEq  = item{itemLessEq, 0, "<="}
	tGtEq  = item{itemGreaterEq, 0, ">="}
	tNotEq = item{itemNotEq, 0, "!="}
	tEq    = item{itemEq, 0, "=="}
	tPlus  = item{itemPlus, 0, "+"}
	tMinus = item{itemMinus, 0, "-"}
	tMult  = item{itemMult, 0, "*"}
	tDiv   = item{itemDiv, 0, "/"}
	tMod   = item{itemMod, 0, "%"}
)

var lexTests = []lexTest{
	{"empty", "", []item{tEOF}},
	{"spaces", " \t\n", []item{tEOF}},
	{"text", `"now is the time"`, []item{{itemString, 0, `"now is the time"`}, tEOF}},
	{"operators", "! && || < > <= >= == != + - * / %", []item{
		tNot,
		tAnd,
		tOr,
		tLt,
		tGt,
		tLtEq,
		tGtEq,
		tEq,
		tNotEq,
		tPlus,
		tMinus,
		tMult,
		tDiv,
		tMod,
		tEOF,
	}},
	{"numbers", "1 02 0x14 7.2 1e3 1.2e-4", []item{
		{itemNumber, 0, "1"},
		{itemNumber, 0, "02"},
		{itemNumber, 0, "0x14"},
		{itemNumber, 0, "7.2"},
		{itemNumber, 0, "1e3"},
		{itemNumber, 0, "1.2e-4"},
		tEOF,
	}},
	{"curly brace var", "${My Var}", []item{
		{itemVar, 0, "${My Var}"},
		tEOF,
	}},
	{"curly brace var plus 1", "${My Var} + 1", []item{
		{itemVar, 0, "${My Var}"},
		tPlus,
		{itemNumber, 0, "1"},
		tEOF,
	}},
	{"number plus var", "1 + $A", []item{
		{itemNumber, 0, "1"},
		tPlus,
		{itemVar, 0, "$A"},
		tEOF,
	}},
	// errors
	{"unclosed quote", "\"", []item{
		{itemError, 0, "unterminated string"},
	}},
	{"single quote", "'single quote is invalid'", []item{
		{itemError, 0, "invalid character: '"},
	}},
	{"invalid var", "$", []item{
		{itemError, 0, "incomplete variable"},
	}},
	{"invalid curly var", "${adf sd", []item{
		{itemError, 0, "unterminated variable missing closing }"},
	}},
}

// collect gathers the emitted items into a slice.
func collect(t *lexTest) (items []item) {
	l := lex(t.input)
	for {
		item := l.nextItem()
		items = append(items, item)
		if item.typ == itemEOF || item.typ == itemError {
			break
		}
	}
	return
}

func equal(i1, i2 []item, checkPos bool) bool {
	if len(i1) != len(i2) {
		return false
	}
	for k := range i1 {
		if i1[k].typ != i2[k].typ {
			return false
		}
		if i1[k].val != i2[k].val {
			return false
		}
		if checkPos && i1[k].pos != i2[k].pos {
			return false
		}
	}
	return true
}

func TestLex(t *testing.T) {
	for i, test := range lexTests {
		items := collect(&lexTests[i])
		if !equal(items, test.items, false) {
			t.Errorf("%s: got\n\t%+v\nexpected\n\t%v", test.name, items, test.items)
		}
	}
}

// TestLexerClose verifies that a lexer can be explicitly closed
func TestLexerClose(t *testing.T) {
	// Create a lexer with some input
	lexer := lex("1 + 2")

	// Read one item to verify it's working
	item := lexer.nextItem()
	if item.typ != itemNumber || item.val != "1" {
		t.Errorf("unexpected first item: %v", item)
	}

	// Close the lexer explicitly
	lexer.Close()

	// Verify the lexer's channel closes
	select {
	case _, ok := <-lexer.items:
		if ok {
			t.Fatal("lexer.items channel should be closed after lexer.Close()")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for lexer.items channel to close")
	}
}

// TestParseErrorNoLeak verifies that lexer goroutines are properly terminated when Parse encounters errors
func TestParseErrorNoLeak(t *testing.T) {
	// Count initial goroutines
	initialGoroutines := runtime.NumGoroutine()

	// Create several trees with parsing errors to check for leaks
	for i := 0; i < 10; i++ {
		tree := New()
		input := "invalid expression with $"
		err := tree.Parse(input)

		// Verify that Parse returned an error
		if err == nil {
			t.Fatal("expected error but got nil")
		}

		// Verify that tree.lex is nil after an error
		if tree.lex != nil {
			t.Fatal("tree.lex was not set to nil after error")
		}
	}

	// Poll for goroutine count to stabilize
	deadline := time.Now().Add(500 * time.Millisecond)
	var finalGoroutines int

	for time.Now().Before(deadline) {
		finalGoroutines = runtime.NumGoroutine()
		// If we're close to the initial count, we can exit early
		if finalGoroutines <= initialGoroutines+2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Check if we've leaked goroutines (with a small buffer for normal variations)
	if finalGoroutines > initialGoroutines+5 {
		t.Fatalf("Goroutine leak detected: started with %d goroutines, ended with %d (difference of %d)",
			initialGoroutines, finalGoroutines, finalGoroutines-initialGoroutines)
	}
}
