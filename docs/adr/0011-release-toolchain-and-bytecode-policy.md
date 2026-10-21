# ADR 0011: Release, toolchain, and bytecode compatibility policy

- **Status:** Accepted
- **Date:** 2026-10-21
- **Relationship:** Defines the release process and support policy for the CLI version, Go toolchain, and independently versioned JSON bytecode format.

## Context

The repository embeds version `0.1.0` but has no public Git tag or GitHub Release. The changelog's prototype baseline must not be mistaken for a published release. The bytecode writer and strict reader currently use format version 9, and the reader rejects older artifacts rather than migrating them. The previous Go 1.23 minimum is outside the Go project's currently supported release window.

## Decision

### 1. Product version and releases

- `cmd/aster/VERSION` is the source-controlled product version displayed by the CLI. Public release tags use `vMAJOR.MINOR.PATCH`; the tag without its leading `v` must equal the embedded version at that tag.
- Use Semantic Versioning for published product versions. While below 1.0.0, the product and serialized formats remain prototype-stage; a `0.y` minor release may include incompatible changes.
- Keep `CHANGELOG.md`'s `[Unreleased]` section at the top. Before a release, move the included entries into a dated versioned section and leave a fresh `[Unreleased]` section.
- Publish a Git tag and GitHub Release only after the exact release commit passes CI. The tag, embedded version, changelog section, release notes, and any assets must refer to the same version and commit.
- A changelog heading or embedded version string does not prove a release was published. Prototype history remains explicitly marked unpublished unless a corresponding public tag exists.
- Product version and bytecode format version are independent; changing one does not imply changing the other.

### 2. JSON bytecode compatibility

- `internal/bytecode.FormatVersion` is the authoritative wire-format version; the current version is 9.
- The writer emits only the current format and the reader accepts only an exact version match. Older and newer formats are rejected. No automatic migration or backward-compatibility promise is provided for bytecode files in the 0.x prototype.
- Bump the format version when serialized schema or instruction/metadata interpretation changes so that an older reader could misinterpret it or a newer reader cannot safely treat it as the previous format. Explain the reason in the changelog and language specification.
- Every format bump must test a current-format round trip, rejection of at least the immediately previous version, rejection of unsupported future versions, and malformed-format validation. Run VM/Native parity tests when runtime semantics are affected.
- After a format bump, users must recompile source with a matching Aster release. Do not describe bytecode files as portable across Aster releases unless that compatibility is implemented and tested.

### 3. Go toolchain support

- The minimum supported Go version follows the oldest upstream-supported release line. The Go project states that each release remains supported until two newer major releases exist: [Go Release History — Release Policy](https://go.dev/doc/devel/release).
- As of 2026-10-21, Go 1.27 is current and Go 1.26 is the previous supported line. Aster's supported minimum is therefore Go 1.26.
- The `go.mod` directive, CI matrix, English/Japanese READMEs, toolchain-unavailable diagnostics, and active CI validation instructions must agree on Go 1.26. CI tests `1.26.x` and the latest stable release.
- Revisit the minimum when upstream support moves forward. Updating the minimum requires synchronizing the module directive, CI, user-facing requirements and diagnostics, and active validation instructions. Older versions may happen to work but are not a support or security-maintenance commitment.

### 4. Release readiness checklist

1. Review the full diff from the last public tag, or the initial repository baseline if no tag exists.
2. Ensure the embedded version and changelog identify the intended release.
3. Pass the minimum-supported and stable Go CI jobs, including `go test ./...`, race detection, `go vet ./...`, CLI build, examples, and the 32-bit safety check.
4. Verify external `go install github.com/musashimusashino666/aster/cmd/aster@<tag-or-revision>` and the resulting `aster version`.
5. Re-check the bytecode format and state artifact compatibility limitations in the release notes.
6. Only then create the `vX.Y.Z` tag and corresponding GitHub Release.

This ADR defines release preparation and policy; it does not itself create a tag or publish a release.

## Consequences

- `0.1.0` remains the embedded development version until the release commit is prepared; its presence in source is not evidence of publication.
- Bytecode v9 remains strict and non-migrating. Its format version changes independently of the product version.
- Go 1.23 is no longer in the supported CI matrix; Go 1.26.x becomes the minimum continuously tested line.

## Validation

- Verify `go.mod`, CI, READMEs, diagnostics, and active validation instructions agree on Go 1.26.
- Verify the changelog distinguishes unreleased work from an unpublished prototype baseline.
- Keep tests confirming that bytecode v8 and unsupported future versions are rejected by the v9 reader.
