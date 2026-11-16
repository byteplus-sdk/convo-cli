// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import "convo-cli/internal/officialskills"

// EmbeddedSkills keeps official scene skills in the versioned release binary
// so future local inspection/brand-index serving uses the exact CLI content.
var EmbeddedSkills = officialskills.Current()
