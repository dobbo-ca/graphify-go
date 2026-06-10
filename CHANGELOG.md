
### ♻️ Refactor

- Drop unused null-label carry fields

### 🐛 Bug Fixes

- Never fabricate null-label id when inherited context is partial
- Restore exact null-label id for fully-resolved wrapper chains

### 📚 Documentation

- Update changelog for v0.4.0 [skip ci]
- Design spec for cloudposse null-label graph awareness
- Align null-label spec with current main (module-source linking exists)
- Implementation plan for null-label graph awareness
- Document null-label tagging and computed-name search

### 🔧 Miscellaneous Tasks

- Regenerate knowledge graph [skip ci]

### 🚀 Features

- Tag cloudposse null-label modules in the TF graph
- Model TF context= inheritance as inherits_context edges
- Add searchable ComputedName node field for TF labels
- Reconstruct cloudposse null-label computed names (single block)
- Capture null-label var-refs and module invocation args
- Resolve null-label names across local module chains
- Include inherits_context in affected blast radius

### 🧪 Testing

- Null-label computed name end-to-end + fixture

