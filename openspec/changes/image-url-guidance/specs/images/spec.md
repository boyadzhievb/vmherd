## MODIFIED Requirements

### Requirement: Verified during download
vmherd SHALL find the vendor's checksum file in the image's directory listing (names containing SUM, SHA256 or SHA512), SHALL compute the SHA-256 or SHA-512 hash while downloading, and SHALL move the file into the cache only when the hash matches. Lines for other files, including near-matches of the file name, and hashes that are not 64 or 128 hex digits SHALL never be used. A directory listing or checksum file larger than 1 MiB SHALL fail the download instead of being read cut off.

#### Scenario: Checksum discovery
- **WHEN** the vendor directory links `SHA256SUMS` or `<file>.SHA256` (Arch style)
- **THEN** vmherd finds the right checksum file and skips unsupported algorithms such as MD5
- **Test:** internal/image/TestExtractChecksumLinksArchStyle, internal/image/TestExtractChecksumLinksSumsLists, internal/image/TestDiscoverChecksumArchStyle, internal/image/TestExtractChecksumLinksSkipsUnsupportedAlgorithms

#### Scenario: No checksum available
- **WHEN** the directory links no checksum file, or none contains the image's hash
- **THEN** the download fails and says it can't be verified
- **Test:** internal/image/TestDiscoverChecksumErrorMessages

#### Scenario: Checksum file formats
- **WHEN** the checksum file uses `sha256sum` or BSD format, has comments, blank lines or a manifest entry for a similar name
- **THEN** only the exact file name's hash is taken
- **Test:** internal/image/TestParseChecksumSha256sumFormat, internal/image/TestParseChecksumBSDFormat, internal/image/TestParseChecksumNotFound, internal/image/TestParseChecksumEmptyContent, internal/image/TestParseChecksumCommentOnly, internal/image/TestParseChecksumWhitespaceLines, internal/image/TestParseChecksumExactMatchIgnoresManifest, internal/image/TestParseChecksumExactMatchPreventsSubstring

#### Scenario: Hash mismatch
- **WHEN** the downloaded bytes don't match the vendor hash
- **THEN** nothing is added to the cache and the error shows both hashes and suggests `vmherd pull` again
- **Test:** none yet — can use the TLS test server TestCheckImageURLAvailable already uses

#### Scenario: Malformed hash or oversize vendor file
- **WHEN** a checksum line's hash is not 64 or 128 hex digits (e.g. it holds terminal escape codes), or the listing or checksum file is larger than 1 MiB
- **THEN** that line is not used, and an oversize file fails the download saying it is larger than the limit
- **Test:** internal/image/TestParseChecksumLineRejectsNonHexHash, internal/image/TestOversizeVendorResponseFails
