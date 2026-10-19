# ADR 0002: Checked arithmetic in the prototype

- Status: accepted for Aster 0.1
- Date: 2026-10-19

## Context

Integer wraparound and division-by-zero behavior should not vary silently between execution backends. The prototype prioritizes diagnosable failures over silent signed-integer overflow.

## Decision

Signed `i64` overflow, integer division/remainder by zero, floating division by zero, and non-finite floating arithmetic results produce runtime errors in the VM and native backend.

## Consequences

- The VM and native backend can share a clear behavior contract.
- Checked helper calls add overhead in generated native code.
- The semantics differ from raw Go arithmetic and need to be documented as Aster language behavior.
