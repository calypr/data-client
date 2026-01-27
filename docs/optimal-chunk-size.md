

# Engineering note — Optimal Chunk Size Calculation for Multipart Uploads

## OLD:
 optimalChunkSize determines the ideal chunk/part size for multipart upload based on file size.
 The chunk size (also known as "message size" or "part size") affects upload performance and
 must comply with S3 constraints.

 Calculation logic:
   - For files ≤ 512 MB: Returns 32 MB chunks for optimal performance
   - For files > 512 MB: Calculates fileSize/maxMultipartParts, with minimum of 5 MB
   - Enforces minimum of 5 MB (S3 requirement for all parts except the last)
   - Rounds up to nearest MB for alignment

 This results in:
   - Files ≤ 512 MB: 32 MB chunks
   - Files 512 MB - ~49 GB: 5 MB chunks (minimum enforced)
     The ~49 GB threshold (10,000 parts × 5 MB) is where files exceed S3's
     10,000 part limit when using the minimum chunk size
   - Files > ~49 GB: Dynamically calculated to stay under 10,000 parts

 Examples:
   - 100 MB file  → 32 MB chunks (4 parts)
   - 1 GB file    → 5 MB chunks (~205 parts)
   - 10 GB file   → 5 MB chunks (~2,048 parts)
   - 50 GB file   → 6 MB chunks (~8,534 parts)
   - 100 GB file  → 11 MB chunks (~9,310 parts)
   - 1 TB file    → 105 MB chunks (~9,987 parts)

## NEW

OptimalChunkSize determines the ideal chunk/part size for multipart upload based on file size.
The chunk size (also known as "message size" or "part size") affects upload performance and
must comply with S3 constraints.

Calculation logic:
  - For files ≤ 100 MB: Returns the file size itself (single PUT, no multipart)
  - For files > 100 MB and ≤ 1 GB: Returns 10 MB chunks
  - For files > 1 GB and ≤ 10 GB: Scales linearly between 25 MB and 128 MB
  - For files > 10 GB and ≤ 100 GB: Returns 256 MB chunks
  - For files > 100 GB: Scales linearly between 512 MB and 1024 MB (capped at 1 TB for ratio purposes)
  - All chunk sizes are rounded down to the nearest MB
  - Minimum chunk size is 1 MB (for zero or negative input)

This results in:
  - Files ≤ 100 MB: Single PUT upload
  - Files 100 MB - 1 GB: 10 MB chunks
  - Files 1 GB - 10 GB: 25-128 MB chunks (scaled)
  - Files 10 GB - 100 GB: 256 MB chunks
  - Files > 100 GB: 512-1024 MB chunks (scaled)

Examples:
  - 100 MB file  → 100 MB chunk (1 part, single PUT)
  - 500 MB file  → 10 MB chunks (50 parts)
  - 1 GB file    → 10 MB chunks (103 parts)
  - 5 GB file    → 70 MB chunks (74 parts, scaled)
  - 10 GB file   → 128 MB chunks (80 parts)
  - 50 GB file   → 256 MB chunks (200 parts)
  - 100 GB file  → 256 MB chunks (400 parts)
  - 500 GB file  → 739 MB chunks (693 parts, scaled)
  - 1 TB file    → 1024 MB chunks (1024 parts)

### Testing


```bash
go test ./client/upload -run '^TestOptimalChunkSize$' -v

```

Purpose
- Validate `OptimalChunkSize` behavior and return values (chunk size and number of parts) across thresholds, boundaries and scaled ranges.

Key behavior to assert
1. Input type and units: sizes are `int64` bytes; tests should use `common.MB` / `common.GB` constants.
2. Parts calculation: `parts = ceil(fileSize / chunk)`; `fileSize == 0` returns `parts == 0`.
3. Scaling: scaled ranges are linear, rounded **down** to the nearest MB and clamped to range.
4. Minimum chunk clamp: result is at least `1 MB`.
5. Boundary semantics: implementation uses `<=` and some ranges start at `X + 1` — include exact, \-1 and \+1 byte checks.

Parameterized test cases (file size ⇒ expected chunk ⇒ expected parts)
1. `0` bytes
    - chunk: `1 MB` (fallback)
    - parts: `0`

2. `1 MB`
    - chunk: `1 MB` (<= 100 MB)
    - parts: `1`

3. `100 MB`
    - chunk: `100 MB` (<= 100 MB)
    - parts: `1`

4. `100 MB + 1 B`
    - chunk: `10 MB` (> 100 MB - <= 1 GB)
    - parts: ceil((100 MB + 1 B) / 10 MB) = `11`

5. `500 MB`
    - chunk: `10 MB`
    - parts: `50`

6. `1 GB` (1024 MB)
    - chunk: `10 MB` (<= 1 GB)
    - parts: ceil(1024 / 10) = `103`

7. `1 GB + 1 B`
    - chunk: `25 MB` (start of 1 GB - 10 GB scaled range)
    - parts: ceil((1024 MB + 1 B) / 25 MB) = `41`

8. `5 GB` (5120 MB)
    - chunk: linear between `25 MB` and `128 MB` → ≈ `70 MB` (rounded down)
    - parts: ceil(5120 / 70) = `74`

9. `10 GB` (10240 MB)
    - chunk: `128 MB` (end of 1 GB - 10 GB scaled range)
    - parts: `80`

10. `10 GB + 1 B`
    - chunk: `256 MB` (> 10 GB - <= 100 GB fixed)
    - parts: ceil((10240 MB + 1 B) / 256 MB) = `41`

11. `50 GB` (51200 MB)
    - chunk: `256 MB`
    - parts: `200`

12. `100 GB` (102400 MB)
    - chunk: `256 MB`
    - parts: `400`

13. `100 GB + 1 B`
    - chunk: `512 MB` (start of > 100 GB scaled range)
    - parts: ceil((102400 MB + 1 B) / 512 MB) = `201`

14. `500 GB` (512000 MB)
    - chunk: linear between `512 MB` and `1024 MB` → ≈ `739 MB` (rounded down)
    - parts: ceil(512000 / 739) = `693`

15. `1 TB` (1024 GB = 1,048,576 MB) — note: use project units consistently
    - chunk: `1024 MB` (max of scaled range)
    - parts: 1,048,576 / 1024 = `1024`

Test design notes (concise)
1. Use table-driven subtests in `client/upload/utils_test.go`. Include fields: name, `fileSize int64`, `wantChunk int64`, `wantParts int64`.
2. For scaled cases assert: MB alignment, clamped to min/max, and exact `wantParts`. Use integer arithmetic for parts.
3. Add explicit boundary triples for each threshold: exact, -1 byte, +1 byte.
4. Include negative and zero cases to verify fallback behavior.
5. Keep tests deterministic and fast (no external deps).

Execution
- Run from repo root: `go test ./client/upload -v`
- Run single test: `go test ./client/upload -run '^TestOptimalChunkSize$' -v`