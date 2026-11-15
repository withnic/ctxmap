package parser

import "testing"

func TestTSFileDoc(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"line comment before imports",
			"// Settings persistence and IPC handlers.\nimport x from 'y'\n\nexport const a = 1\n",
			"Settings persistence and IPC handlers."},
		{"jsdoc with @file tag",
			"/**\n * @file Avatar loading.\n * Second line.\n */\n\nexport function f() {}\n",
			"Avatar loading.\nSecond line."},
		{"license header is skipped",
			"/*\n * Copyright 2024 Acme Inc.\n * Licensed under the MIT License.\n */\n\n// Real description.\nimport a from 'a'\n",
			"Real description."},
		{"SPDX and tool directives are skipped",
			"// SPDX-License-Identifier: MIT\n/* eslint-disable no-console */\n// @ts-nocheck\n\n/** The description. */\n\nexport {}\n",
			"The description."},
		{"shebang and directive prologue",
			"#!/usr/bin/env node\n'use client';\n\n// CLI entry point.\n\nmain()\n",
			"CLI entry point."},
		{"comment attached to the first declaration is its doc",
			"/** Adds numbers. */\nexport function add(a: number) { return a }\n",
			""},
		{"blank line separates file doc from the declaration",
			"// File role.\n\nexport function add(a: number) { return a }\n",
			"File role."},
		{"code before any comment",
			"import a from 'a'\n// not a file doc\nexport const b = a\n",
			""},
		{"only boilerplate",
			"/* eslint-disable */\nexport const a = 1\n",
			""},
		{"utf-8 BOM",
			utf8BOM + "// With BOM.\n\nexport const a = 1\n",
			"With BOM."},
		{"empty file", "", ""},
	}
	for _, c := range cases {
		fi, err := ParseTS("x.ts", []byte(c.src))
		if err != nil {
			t.Fatal(err)
		}
		if fi.FileDoc != c.want {
			t.Errorf("%s: FileDoc = %q, want %q", c.name, fi.FileDoc, c.want)
		}
	}

	// the declaration keeps its own doc when the comment is attached to it
	fi, _ := ParseTS("x.ts", []byte("/** Adds numbers. */\nexport function add(a: number) { return a }\n"))
	if len(fi.Symbols) != 1 || fi.Symbols[0].Doc != "Adds numbers." {
		t.Errorf("declaration doc lost: %+v", fi.Symbols)
	}
}

func TestGoFileDoc(t *testing.T) {
	src := "// Copyright 2024 Acme. All rights reserved.\n\n// Package demo does demo things.\n//\n// # Usage\n//\n// More text.\npackage demo\n"
	fi, err := ParseGo("doc.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if want := "Package demo does demo things.\n\n# Usage\n\nMore text."; fi.FileDoc != want {
		t.Errorf("FileDoc = %q, want %q (license header must not be included)", fi.FileDoc, want)
	}
	if Intro(fi.FileDoc) != "Package demo does demo things." {
		t.Errorf("Intro should stop before the first heading: %q", Intro(fi.FileDoc))
	}
	noDoc, _ := ParseGo("a.go", []byte("// Copyright 2024 Acme.\n\npackage demo\n"))
	if noDoc.FileDoc != "" {
		t.Errorf("license header is not a package doc: %q", noDoc.FileDoc)
	}
}

func TestIntroCap(t *testing.T) {
	long := ""
	for i := 0; i < 40; i++ {
		long += "line\n"
	}
	if got := Intro(long); len(got) != len("line")*maxIntroLines+maxIntroLines-1 {
		t.Errorf("Intro should keep at most %d lines, got %q", maxIntroLines, got)
	}
}
