# Standard library boundary

The 0.1 implementation deliberately contains only three built-ins: `print`, `len`, and `assert`. Their behavior is implemented in both the VM and the native code-generation runtime.

The standard library is intentionally not represented as fully implemented packages. Future candidates include:

- `std.string`: Unicode-aware text operations
- `std.collections`: slices, maps, sets (core `Array<T>` exists, but the collection library is not implemented)
- `std.math`: checked integer helpers and numeric utilities
- `std.fs`: file and path operations
- `std.net`: HTTP and sockets
- `std.json`: JSON parsing and serialization
- `std.time`: durations, clocks, time zones
- `std.test`: assertions and test discovery

Before adding the module system, these are design targets only and are not importable modules.
