package parser

import (
	"strings"
	"testing"
)

const tsSample = `import React from 'react';
import {
  a,
  b,
} from './lib';
export { x } from "../x";
const lazy = () => import('./lazy');
const fs = require('fs');

/**
 * Props for App.
 */
export interface Props {
  title: string; // { not a brace
}

export type Mode =
  | 'a'
  | 'b';

/** Adds. */
export async function add(a: number): Promise<{ sum: number }> {
  const re = /[{]/g;
  const s = ` + "`${a} {`" + `;
  return { sum: a };
}

export const App = ({ title }: Props) => {
  return title;
};

export const CONFIG = {
  a: "}",
};

export default class Store extends Base {
  @dec() items: string[] = [];
  private secret = 1;
  #hidden = 2;
  async fetch(id: string): Promise<void> {
    if (id) { return; }
  }
}

function internal() { return 1 }
`

func TestParseTS(t *testing.T) {
	fi, err := ParseTS("app.ts", []byte(tsSample))
	if err != nil {
		t.Fatal(err)
	}
	wantImports := []string{"react", "./lib", "../x", "fs", "./lazy"}
	if strings.Join(fi.Imports, ",") != strings.Join(wantImports, ",") {
		t.Errorf("imports = %v, want %v", fi.Imports, wantImports)
	}

	got := map[string]Symbol{}
	for _, s := range fi.Symbols {
		got[s.Name] = s
	}
	cases := []struct {
		name, sig string
		exported  bool
	}{
		{"Props", "export interface Props {\n  title: string;\n}", true},
		{"Mode", "export type Mode =\n  | 'a'\n  | 'b';", true},
		{"add", "export async function add(a: number): Promise<{ sum: number }>", true},
		{"App", "export const App = ({ title }: Props) =>", true},
		{"CONFIG", "export const CONFIG = { …", true},
		{"Store", "export default class Store extends Base", true},
		{"internal", "function internal()", false},
	}
	for _, c := range cases {
		s, ok := got[c.name]
		if !ok {
			t.Errorf("%s: not found (got %v)", c.name, fi.Symbols)
			continue
		}
		if s.Signature != c.sig {
			t.Errorf("%s: sig = %q, want %q", c.name, s.Signature, c.sig)
		}
		if s.Exported != c.exported {
			t.Errorf("%s: exported = %v", c.name, s.Exported)
		}
	}
	if d := got["Props"].Doc; d != "Props for App." {
		t.Errorf("Props doc = %q", d)
	}
	if s := got["add"]; s.Doc != "Adds." || s.Start != 21 || s.End != 26 {
		t.Errorf("add: doc=%q lines=%d-%d", s.Doc, s.Start, s.End)
	}

	var members []string
	for _, m := range got["Store"].Members {
		if m.Exported {
			members = append(members, m.Name+"|"+m.Signature)
		}
	}
	want := "items|items: string[] = [],fetch|async fetch(id: string): Promise<void>"
	if strings.Join(members, ",") != want {
		t.Errorf("members = %v", members)
	}
}

func TestSummary(t *testing.T) {
	if s := Summary("Foo does x. And y.\nMore."); s != "Foo does x." {
		t.Errorf("got %q", s)
	}
	if s := Summary("Line one\ncontinues here\n\nSecond paragraph."); s != "Line one continues here" {
		t.Errorf("got %q", s)
	}
}
