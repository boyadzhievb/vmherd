## MODIFIED Requirements

### Requirement: Example Herdfile
`vmherd init` SHALL write an example Herdfile that loads without problems, and SHALL refuse to overwrite an existing one. Above `images:`, the example SHALL have a comment that says where to find an image URL and links to the image guide `docs/images.md` on the vmherd GitHub page.

#### Scenario: Example is valid
- **WHEN** the example Herdfile is generated
- **THEN** it is valid YAML and passes validation
- **Test:** internal/config/TestExampleHerdfileIsValidYAML, internal/config/TestExampleHerdfilePassesValidation, internal/config/TestBuiltInImagesAreValid

#### Scenario: Herdfile exists
- **WHEN** the user runs `vmherd init` where a Herdfile exists
- **THEN** it fails with "Herdfile already exists" and leaves the file unchanged
- **Test:** none yet — cmd/ code, needs M2.3

#### Scenario: Example points to the image URL guide
- **WHEN** the example Herdfile is generated
- **THEN** the lines directly above `images:` are a comment that says to add the vendor's image URL for other images and gives the link `https://github.com/boyadzhievb/vmherd/blob/main/docs/images.md`, and the Herdfile still passes validation
- **Test:** internal/config/TestExampleHerdfilePointsToImageURLGuide

#### Scenario: The link reaches the image guide
- **WHEN** the docs are changed
- **THEN** `docs/images.md` still exists at the path the example Herdfile's link names
- **Test:** test/tooling/TestImageURLGuideLinkMatchesFile

#### Scenario: The README references the guide
- **WHEN** a user reads the README's image section or the `images` row of its Herdfile reference
- **THEN** both link to `docs/images.md`
- **Test:** test/tooling/TestReadmeLinksImageGuide
