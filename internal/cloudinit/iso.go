package cloudinit

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// isoSectorSize is the standard sector size for ISO 9660 images.
const isoSectorSize = 2048

// isoSystemAreaSectors is the number of reserved sectors at the start of the image.
const isoSystemAreaSectors = 16

// isoPrimaryVolumeDescriptorType identifies a Primary Volume Descriptor.
const isoPrimaryVolumeDescriptorType = 1

// isoTerminatorType identifies the Volume Descriptor Set Terminator.
const isoTerminatorType = 255

// isoVolumeDescriptorVersion is the version field for all volume descriptors.
const isoVolumeDescriptorVersion = 1

// isoFileFlags is the flag byte for a regular file (no directory, no hidden).
const isoFileFlags = 0

// isoDirectoryFlag marks a directory record as a directory.
const isoDirectoryFlag = 2

// isoStandardIdentifier is the magic string in every volume descriptor.
var isoStandardIdentifier = [5]byte{'C', 'D', '0', '0', '1'}

// rockRidgeSignatureRR identifies the Rock Ridge "RR" extension marker.
var rockRidgeSignatureRR = [2]byte{'R', 'R'}

// rockRidgeSignatureNM identifies the Rock Ridge Alternate Name entry.
var rockRidgeSignatureNM = [2]byte{'N', 'M'}

// rockRidgeSignaturePX identifies the Rock Ridge POSIX File Attributes entry.
var rockRidgeSignaturePX = [2]byte{'P', 'X'}

// rockRidgeVersion is the version byte for all Rock Ridge system use entries.
const rockRidgeVersion = 1

// rockRidgeRRFlagsPX declares that PX (POSIX attributes) is present.
const rockRidgeRRFlagsPX = 0x01

// rockRidgeRRFlagsPXNM declares that PX and NM (alternate name) are present.
const rockRidgeRRFlagsPXNM = 0x09

// isoFile represents a file to include in the ISO image.
type isoFile struct {
	name    string // the filename as it appears on the ISO (Rock Ridge alternate name)
	content []byte // the file data
}

// buildISO9660 creates a minimal ISO 9660 image with Rock Ridge extensions.
// The volume label is set on the Primary Volume Descriptor. All files are placed
// in the root directory. The image is built entirely in memory.
func buildISO9660(volumeLabel string, files []isoFile) ([]byte, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("cannot build ISO: no files provided")
	}

	now := time.Now().UTC()

	pathTableSector := uint32(isoSystemAreaSectors + 2) // sector 18
	rootDirSector := pathTableSector + 1                // sector 19

	rootDirData := buildRootDirectory(files, rootDirSector, now)
	rootDirSectors := sectorsNeeded(len(rootDirData))

	fileDataStart := rootDirSector + uint32(rootDirSectors)
	fileLocations := assignFileLocations(files, fileDataStart)

	patchDirectoryFileLocations(rootDirData, files, fileLocations)

	pathTable := buildPathTable(rootDirSector)

	totalSectors := fileLocations[len(fileLocations)-1].sector +
		uint32(sectorsNeeded(len(files[len(files)-1].content)))

	var buf bytes.Buffer

	buf.Write(make([]byte, isoSystemAreaSectors*isoSectorSize))

	rootDirSizeAligned := uint32(rootDirSectors) * isoSectorSize
	writePrimaryVolumeDescriptor(&buf, volumeLabel, totalSectors, rootDirSector,
		rootDirSizeAligned, pathTableSector, uint32(len(pathTable)), now)

	writeTerminator(&buf)

	writePadded(&buf, pathTable)

	writePadded(&buf, rootDirData)

	for i, file := range files {
		if uint32(buf.Len()/isoSectorSize) != fileLocations[i].sector {
			return nil, fmt.Errorf("ISO layout error: expected file %q at sector %d, but buffer is at sector %d",
				file.name, fileLocations[i].sector, buf.Len()/isoSectorSize)
		}
		writePadded(&buf, file.content)
	}

	return buf.Bytes(), nil
}

// fileLocation tracks where a file's data starts in the image.
type fileLocation struct {
	sector uint32
	size   uint32
}

// assignFileLocations computes the starting sector for each file's data.
func assignFileLocations(files []isoFile, startSector uint32) []fileLocation {
	locations := make([]fileLocation, len(files))
	currentSector := startSector
	for i, file := range files {
		locations[i] = fileLocation{sector: currentSector, size: uint32(len(file.content))}
		currentSector += uint32(sectorsNeeded(len(file.content)))
	}
	return locations
}

// sectorsNeeded returns the number of 2048-byte sectors needed for the given byte count.
func sectorsNeeded(byteCount int) int {
	if byteCount == 0 {
		return 1
	}
	return (byteCount + isoSectorSize - 1) / isoSectorSize
}

// writePadded writes data to the buffer, padding to a sector boundary with zeros.
func writePadded(buf *bytes.Buffer, data []byte) {
	buf.Write(data)
	remainder := len(data) % isoSectorSize
	if remainder != 0 {
		buf.Write(make([]byte, isoSectorSize-remainder))
	}
}

// writePrimaryVolumeDescriptor writes the PVD at the current buffer position.
func writePrimaryVolumeDescriptor(buf *bytes.Buffer, volumeLabel string,
	totalSectors, rootDirSector, rootDirSize, pathTableSector, pathTableSize uint32,
	now time.Time) {

	sector := make([]byte, isoSectorSize)
	sector[0] = isoPrimaryVolumeDescriptorType
	copy(sector[1:6], isoStandardIdentifier[:])
	sector[6] = isoVolumeDescriptorVersion

	writeStrA(sector[8:40], "")                    // system identifier
	writeStrD(sector[40:72], volumeLabel)          // volume identifier
	writeBothEndian32(sector[80:88], totalSectors) // volume space size

	writeBothEndian16(sector[120:124], 1)             // volume set size
	writeBothEndian16(sector[124:128], 1)             // volume sequence number
	writeBothEndian16(sector[128:132], isoSectorSize) // logical block size

	writeBothEndian32(sector[132:140], pathTableSize)
	binary.LittleEndian.PutUint32(sector[140:144], pathTableSector)
	binary.BigEndian.PutUint32(sector[148:152], pathTableSector)

	writeRootDirRecord(sector[156:190], rootDirSector, rootDirSize, now)

	writeStrD(sector[190:318], volumeLabel) // volume set identifier
	writeStrA(sector[318:446], "")          // publisher identifier
	writeStrA(sector[446:574], "")          // data preparer identifier
	writeStrA(sector[574:702], "VMHERD")    // application identifier

	writeDecDatetime(sector[813:830], now) // volume creation date
	writeDecDatetime(sector[830:847], now) // volume modification date
	writeZeroDatetime(sector[847:864])     // volume expiration date
	writeZeroDatetime(sector[864:881])     // volume effective date

	sector[881] = 1 // file structure version

	buf.Write(sector)
}

// writeTerminator writes the Volume Descriptor Set Terminator.
func writeTerminator(buf *bytes.Buffer) {
	sector := make([]byte, isoSectorSize)
	sector[0] = isoTerminatorType
	copy(sector[1:6], isoStandardIdentifier[:])
	sector[6] = isoVolumeDescriptorVersion
	buf.Write(sector)
}

// writeRootDirRecord writes a 34-byte directory record for the root directory
// into the PVD's root directory record field.
func writeRootDirRecord(dst []byte, sector, dataSize uint32, now time.Time) {
	dst[0] = 34 // record length
	writeBothEndian32(dst[2:10], sector)
	writeBothEndian32(dst[10:18], dataSize)
	writeRecordingDatetime(dst[18:25], now)
	dst[25] = isoDirectoryFlag
	writeBothEndian16(dst[28:32], 1) // volume sequence number
	dst[32] = 1                      // file identifier length
	dst[33] = 0                      // file identifier (root)
}

// buildRootDirectory constructs the root directory data with '.' and '..' entries
// followed by one entry per file. Each file entry includes Rock Ridge extensions.
// File data locations are set to 0 initially and patched by patchDirectoryFileLocations.
func buildRootDirectory(files []isoFile, rootDirSector uint32, now time.Time) []byte {
	var dir bytes.Buffer

	writeDirEntry(&dir, 0x00, rootDirSector, 0, now, true, nil) // "." (self)
	writeDirEntry(&dir, 0x01, rootDirSector, 0, now, true, nil) // ".." (parent)

	for _, file := range files {
		iso9660Name := toISO9660Name(file.name)
		suEntries := buildRockRidgeFileEntries(file.name)
		writeDirEntry(&dir, 0, 0, uint32(len(file.content)), now, false, &fileEntry{
			iso9660Name:    iso9660Name,
			systemUseBytes: suEntries,
		})
	}

	dirBytes := dir.Bytes()
	sectorAlignedSize := uint32(sectorsNeeded(len(dirBytes)) * isoSectorSize)
	writeBothEndian32(dirBytes[10:18], sectorAlignedSize)
	dotRecordLen := int(dirBytes[0])
	writeBothEndian32(dirBytes[dotRecordLen+10:dotRecordLen+18], sectorAlignedSize)
	return dirBytes
}

// fileEntry carries the ISO 9660 name and Rock Ridge system use data for a file.
type fileEntry struct {
	iso9660Name    string
	systemUseBytes []byte
}

// writeDirEntry writes a single directory record. For '.' and '..', identifier is
// the special byte (0x00 or 0x01). For files, entry carries the name and Rock Ridge data.
func writeDirEntry(buf *bytes.Buffer, specialID byte, sector, dataSize uint32,
	now time.Time, isDir bool, entry *fileEntry) {

	var identifier []byte
	var systemUse []byte

	if entry != nil {
		identifier = []byte(entry.iso9660Name)
		systemUse = entry.systemUseBytes
	} else {
		identifier = []byte{specialID}
	}

	identifierLen := len(identifier)
	baseLen := 33 + identifierLen
	if identifierLen%2 == 0 {
		baseLen++ // padding byte for even-length identifiers
	}

	if entry == nil {
		switch specialID {
		case 0x00:
			systemUse = buildRockRidgeDirEntries(true)
		case 0x01:
			systemUse = buildRockRidgeDirEntries(false)
		}
	}

	recordLen := baseLen + len(systemUse)
	if recordLen > 255 {
		panic(fmt.Sprintf("directory record too large (%d bytes) for file %q", recordLen, identifier))
	}

	record := make([]byte, recordLen)
	record[0] = byte(recordLen)
	writeBothEndian32(record[2:10], sector)
	writeBothEndian32(record[10:18], dataSize)
	writeRecordingDatetime(record[18:25], now)

	if isDir {
		record[25] = isoDirectoryFlag
	} else {
		record[25] = isoFileFlags
	}

	writeBothEndian16(record[28:32], 1) // volume sequence number
	record[32] = byte(identifierLen)
	copy(record[33:33+identifierLen], identifier)

	if len(systemUse) > 0 {
		copy(record[baseLen:], systemUse)
	}

	buf.Write(record)
}

// patchDirectoryFileLocations overwrites the extent location fields in the root
// directory with the actual file sector locations.
func patchDirectoryFileLocations(dirData []byte, files []isoFile, locations []fileLocation) {
	offset := 0
	fileIndex := 0
	for offset < len(dirData) {
		recordLen := int(dirData[offset])
		if recordLen == 0 {
			break
		}
		idLen := int(dirData[offset+32])
		if idLen == 1 && (dirData[offset+33] == 0x00 || dirData[offset+33] == 0x01) {
			// '.' or '..' — skip
		} else {
			writeBothEndian32(dirData[offset+2:offset+10], locations[fileIndex].sector)
			writeBothEndian32(dirData[offset+10:offset+18], locations[fileIndex].size)
			fileIndex++
		}
		offset += recordLen
	}
}

// toISO9660Name converts a filename to a valid ISO 9660 Level 1 identifier.
// Hyphens become underscores, lowercase becomes uppercase, and names are
// truncated to 8 characters with a ";1" version suffix.
func toISO9660Name(name string) string {
	upper := strings.ToUpper(name)
	var cleaned strings.Builder
	for _, ch := range upper {
		switch {
		case ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9', ch == '_':
			cleaned.WriteRune(ch)
		default:
			cleaned.WriteRune('_')
		}
	}
	result := cleaned.String()
	if len(result) > 8 {
		result = result[:8]
	}
	return result + ";1"
}

// buildRockRidgeDirEntries returns Rock Ridge system use entries for a
// directory's "." or ".." record: RR (extension marker) and PX (POSIX
// attributes with mode drwxr-xr-x). includeSP adds the SUSP SP entry that
// declares Rock Ridge is in use — required in the "." entry of the root only.
func buildRockRidgeDirEntries(includeSP bool) []byte {
	var entries bytes.Buffer
	if includeSP {
		entries.Write([]byte{'S', 'P', 7, rockRidgeVersion, 0xBE, 0xEF, 0})
	}
	entries.Write([]byte{rockRidgeSignatureRR[0], rockRidgeSignatureRR[1], 5, rockRidgeVersion, rockRidgeRRFlagsPX})
	var pxEntry [36]byte
	pxEntry[0] = rockRidgeSignaturePX[0]
	pxEntry[1] = rockRidgeSignaturePX[1]
	pxEntry[2] = 36
	pxEntry[3] = rockRidgeVersion
	writeBothEndian32(pxEntry[4:12], 0o40755)
	writeBothEndian32(pxEntry[12:20], 2)
	writeBothEndian32(pxEntry[20:28], 0)
	writeBothEndian32(pxEntry[28:36], 0)
	entries.Write(pxEntry[:])
	return entries.Bytes()
}

// buildRockRidgeFileEntries returns the Rock Ridge system use entries for a file:
// an RR entry (declares which extensions are present) followed by PX (POSIX
// attributes) and NM (alternate name preserving the original filename).
func buildRockRidgeFileEntries(realName string) []byte {
	var entries bytes.Buffer

	// RR entry: declares PX and NM are present
	entries.Write([]byte{rockRidgeSignatureRR[0], rockRidgeSignatureRR[1], 5, rockRidgeVersion, rockRidgeRRFlagsPXNM})

	// PX entry: POSIX file attributes (36 bytes)
	// mode 0100644 (regular file, rw-r--r--), nlinks 1, uid 0, gid 0, serial 0
	var pxEntry [36]byte
	pxEntry[0] = rockRidgeSignaturePX[0]
	pxEntry[1] = rockRidgeSignaturePX[1]
	pxEntry[2] = 36
	pxEntry[3] = rockRidgeVersion
	writeBothEndian32(pxEntry[4:12], 0o100644) // file mode
	writeBothEndian32(pxEntry[12:20], 1)       // number of links
	writeBothEndian32(pxEntry[20:28], 0)       // uid
	writeBothEndian32(pxEntry[28:36], 0)       // gid
	entries.Write(pxEntry[:])

	// NM entry: alternate name preserving case and hyphens
	nameBytes := []byte(realName)
	nmLen := byte(5 + len(nameBytes))
	entries.WriteByte(rockRidgeSignatureNM[0])
	entries.WriteByte(rockRidgeSignatureNM[1])
	entries.WriteByte(nmLen)
	entries.WriteByte(rockRidgeVersion)
	entries.WriteByte(0) // flags: 0 = this entry contains the complete name
	entries.Write(nameBytes)

	return entries.Bytes()
}

// buildPathTable returns the L-type (little-endian) path table with a single root entry.
func buildPathTable(rootDirSector uint32) []byte {
	// Path table entry for root: dir identifier length 1, extended attribute length 0,
	// extent location (LE 4 bytes), parent directory number (LE 2 bytes), identifier 0x00,
	// padding byte.
	entry := make([]byte, 10)
	entry[0] = 1 // directory identifier length
	entry[1] = 0 // extended attribute record length
	binary.LittleEndian.PutUint32(entry[2:6], rootDirSector)
	binary.LittleEndian.PutUint16(entry[6:8], 1) // parent directory number
	entry[8] = 0x00                              // root directory identifier
	entry[9] = 0                                 // padding
	return entry
}

// writeBothEndian32 writes a uint32 in both little-endian and big-endian format
// (ISO 9660 "both-byte" format), requiring 8 bytes.
func writeBothEndian32(dst []byte, value uint32) {
	binary.LittleEndian.PutUint32(dst[0:4], value)
	binary.BigEndian.PutUint32(dst[4:8], value)
}

// writeBothEndian16 writes a uint16 in both little-endian and big-endian format,
// requiring 4 bytes.
func writeBothEndian16(dst []byte, value uint16) {
	binary.LittleEndian.PutUint16(dst[0:2], value)
	binary.BigEndian.PutUint16(dst[2:4], value)
}

// writeStrD writes a D-character string into a fixed-width field, padded with spaces.
func writeStrD(dst []byte, s string) {
	upper := strings.ToUpper(s)
	for i := range dst {
		dst[i] = ' '
	}
	copy(dst, upper)
}

// writeStrA writes an A-character string into a fixed-width field, padded with spaces.
func writeStrA(dst []byte, s string) {
	for i := range dst {
		dst[i] = ' '
	}
	copy(dst, s)
}

// writeRecordingDatetime writes a 7-byte directory record timestamp.
func writeRecordingDatetime(dst []byte, t time.Time) {
	dst[0] = byte(t.Year() - 1900)
	dst[1] = byte(t.Month())
	dst[2] = byte(t.Day())
	dst[3] = byte(t.Hour())
	dst[4] = byte(t.Minute())
	dst[5] = byte(t.Second())
	dst[6] = 0 // GMT offset in 15-minute intervals
}

// writeDecDatetime writes a 17-byte decimal timestamp for the PVD.
func writeDecDatetime(dst []byte, t time.Time) {
	s := fmt.Sprintf("%04d%02d%02d%02d%02d%02d%02d",
		t.Year(), t.Month(), t.Day(),
		t.Hour(), t.Minute(), t.Second(), 0)
	copy(dst[:16], s)
	dst[16] = 0 // GMT offset
}

// writeZeroDatetime writes a 17-byte zero timestamp (unused date field).
func writeZeroDatetime(dst []byte) {
	for i := range dst {
		dst[i] = '0'
	}
	dst[16] = 0
}
