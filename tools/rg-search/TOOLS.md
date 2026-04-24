---
name: rg-search
description: System-level ripgrep bundle for shell search and workspace search.
tags: system
system_bins:
  - name: rg
    path: ./rg
run:
  lifecycle: daemon
  command: ["./rg-search"]
---
