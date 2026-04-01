# restic (simplified) — Reconstruction Spec

## Overview

A simplified backup tool inspired by restic. Uses content-addressable storage with chunking (content-defined chunking), pack files to bundle blobs, an index for lookup, and a local filesystem backend. Supports init, backup, restore, snapshots, and ls commands. Encryption simplified to AES-256-GCM.

## Scope

**IN SCOPE:**
- CLI with commands: init, backup, restore, snapshots, ls, cat
- Content-addressable storage (SHA256 blob IDs)
- Content-defined chunking (Rabin fingerprint or fixed-size splitting)
- Pack file format (multiple blobs per file, header at end)
- In-memory index (blob ID -> pack location)
- Local filesystem backend (2-level directory structure)
- Snapshot metadata (tree reference, paths, timestamp)
- Tree/Node structure (directory listings with file metadata)
- AES-256-GCM encryption (password-derived key via Argon2)
- Deduplication (skip existing blobs)

**OUT OF SCOPE:**
- Cloud backends (S3, B2, Azure, GCS, SFTP, REST)
- Cache system
- FUSE mount
- Prune/forget/check commands
- Lock management
- Compression (zstd)
- Repository repair
- Copy/migrate commands
- Concurrent pack uploading (simplify to sequential)
- Extended attributes and special file types (device, socket, pipe)

**TARGET:** ~1500-2000 LOC Go, ~15 source files

## Architecture

- **Language/Runtime:** Go 1.22+
- **Key dependencies:**
  - `golang.org/x/crypto` (argon2 key derivation)
  - Standard library only for: crypto/aes, crypto/cipher (GCM), crypto/sha256, encoding/json, io/fs

### Directory Structure
```
restic-mini/
  go.mod
  cmd/
    restic-mini/
      main.go             # CLI entry point (cobra or flag-based)
  internal/
    restic/
      id.go               # ID type (SHA256 hash)
      blob.go             # Blob, BlobHandle, BlobType
      config.go           # Repository config
      snapshot.go         # Snapshot metadata
      node.go             # Node (file/dir in tree)
      tree.go             # Tree (directory listing)
    backend/
      backend.go          # Backend interface
      local.go            # Local filesystem backend
      layout.go           # Directory layout (2-level hash)
    repository/
      repository.go       # Repository (save/load blobs, manage packs)
      packer.go           # Pack file writer
      index.go            # In-memory blob index
    crypto/
      crypto.go           # AES-256-GCM encrypt/decrypt, key derivation
    archiver/
      archiver.go         # Backup: walk filesystem, chunk, save
    restorer/
      restorer.go         # Restore: load snapshot, rebuild files
    chunker/
      chunker.go          # Content-defined chunking (Rabin or fixed-size)
  README.md
```

### Design Patterns
- **Content-addressable:** All data identified by SHA256 of content (deduplication)
- **Pack files:** Multiple blobs bundled into single files (reduces backend operations)
- **Append-only:** Repository only grows (no in-place modification)
- **Interface-based backend:** Backend interface allows swapping storage implementations

## Data Flow

### Backup Pipeline
```
CLI: restic-mini backup /path/to/data -r /path/to/repo
  |
  v
archiver.go: Backup(paths)
  |
  ├── Walk filesystem recursively
  │   For each file:
  │     1. Read file contents
  │     2. Split into chunks (content-defined chunking)
  │     3. Hash each chunk -> SHA256 ID
  │     4. Check index: if blob exists, skip (dedup)
  │     5. Encrypt chunk (AES-256-GCM)
  │     6. Add to current pack
  │     7. Record: blob ID -> pack offset
  │   For each directory:
  │     1. Create Node for each child (name, type, mode, mtime, size, content IDs)
  │     2. Serialize directory as Tree (JSON: sorted list of Nodes)
  │     3. Hash Tree -> TreeBlob ID
  │     4. Encrypt and add to pack
  |
  ├── Finalize packs (write pack header, save to backend)
  ├── Save index (blob->pack mappings)
  |
  v
  Create Snapshot { Time, Tree: rootTreeID, Paths, Hostname }
  |
  v
  Encrypt and save snapshot to backend
```

### Restore Pipeline
```
CLI: restic-mini restore SNAPSHOT_ID -r /path/to/repo -t /target/dir
  |
  v
restorer.go: Restore(snapshotID, targetDir)
  |
  ├── Load snapshot from backend, decrypt
  ├── Get root Tree ID
  ├── Recursively:
  │     1. Load Tree blob (decrypt, parse JSON)
  │     2. For each Node in tree:
  │        - If file: load data blobs by Content IDs, decrypt, concatenate, write file
  │        - If dir: create directory, recurse into Subtree
  │        - Restore metadata (permissions, timestamps)
  |
  v
  Files restored to target directory
```

## Core Components

### ID (internal/restic/id.go)
- **Purpose:** SHA256 content hash used as blob identifier
- **Implementation:**
  ```go
  type ID [32]byte

  func Hash(data []byte) ID              // SHA256 hash of data
  func ParseID(s string) (ID, error)     // Parse hex string to ID
  func NewRandomID() ID                  // Random ID (for repo config)
  func (id ID) String() string           // Hex-encoded string
  func (id ID) IsNull() bool             // All zeros check
  func (id ID) Equal(other ID) bool
  func (id ID) MarshalJSON() ([]byte, error)
  func (id *ID) UnmarshalJSON(b []byte) error
  ```

### BlobType (internal/restic/blob.go)
- **Purpose:** Distinguish data blobs from tree blobs
- **Implementation:**
  ```go
  type BlobType uint8
  const (
      DataBlob BlobType = 1
      TreeBlob BlobType = 2
  )

  type BlobHandle struct {
      ID   ID
      Type BlobType
  }

  type Blob struct {
      BlobHandle
      Length uint     // encrypted size in pack
      Offset uint    // offset within pack file
  }

  type PackedBlob struct {
      Blob
      PackID ID      // which pack file contains this blob
  }
  ```

### Node (internal/restic/node.go)
- **Purpose:** Represents a file or directory in the backup
- **Implementation:**
  ```go
  type Node struct {
      Name    string      `json:"name"`
      Type    string      `json:"type"`      // "file" or "dir"
      Mode    os.FileMode `json:"mode"`
      ModTime time.Time   `json:"mtime"`
      Size    uint64      `json:"size"`
      UID     uint32      `json:"uid"`
      GID     uint32      `json:"gid"`
      Content []ID        `json:"content,omitempty"`  // data blob IDs (files)
      Subtree *ID         `json:"subtree,omitempty"`  // tree blob ID (dirs)
  }
  ```

### Tree (internal/restic/tree.go)
- **Purpose:** A directory listing (sorted list of Nodes)
- **Implementation:**
  ```go
  type Tree struct {
      Nodes []*Node `json:"nodes"`
  }

  func (t *Tree) Sort()                              // Sort nodes by name
  func (t *Tree) Marshal() ([]byte, error)           // JSON encode
  func UnmarshalTree(data []byte) (*Tree, error)     // JSON decode
  ```
- **Storage:** Serialized to JSON, hashed (SHA256 -> TreeBlob ID), encrypted, stored in pack

### Snapshot (internal/restic/snapshot.go)
- **Purpose:** Metadata for a backup
- **Implementation:**
  ```go
  type Snapshot struct {
      Time     time.Time `json:"time"`
      Tree     *ID       `json:"tree"`       // root tree blob ID
      Paths    []string  `json:"paths"`
      Hostname string    `json:"hostname"`
      Username string    `json:"username,omitempty"`
      Tags     []string  `json:"tags,omitempty"`
  }
  ```
- **Storage:** JSON-encoded, encrypted, saved as individual file in `snapshots/` directory

### Config (internal/restic/config.go)
- **Purpose:** Repository configuration (created at init)
- **Implementation:**
  ```go
  type Config struct {
      Version uint   `json:"version"`    // always 1
      ID      string `json:"id"`         // random UUID
  }
  ```

### Backend Interface (internal/backend/backend.go)
- **Purpose:** Abstract storage operations
- **Implementation:**
  ```go
  type FileType string
  const (
      ConfigFile   FileType = "config"
      KeyFile      FileType = "keys"
      SnapshotFile FileType = "snapshots"
      DataFile     FileType = "data"
      IndexFile    FileType = "index"
  )

  type Handle struct {
      Type FileType
      Name string
  }

  type Backend interface {
      Save(ctx context.Context, h Handle, data []byte) error
      Load(ctx context.Context, h Handle) ([]byte, error)
      List(ctx context.Context, t FileType) ([]string, error)
      Remove(ctx context.Context, h Handle) error
      Stat(ctx context.Context, h Handle) (int64, error)
      Close() error
  }
  ```

### Local Backend (internal/backend/local.go)
- **Purpose:** Filesystem storage with 2-level directory hashing
- **Directory layout:**
  ```
  repo/
    config                          # repository config (encrypted)
    keys/
      {ID}                          # key file (encrypted with password)
    data/
      {ID[0:2]}/
        {ID}                        # pack files
    snapshots/
      {ID[0:2]}/
        {ID}                        # snapshot files
    index/
      {ID[0:2]}/
        {ID}                        # index files
  ```
- **Save flow:** Write to temp file, fsync, rename to final path

### Crypto (internal/crypto/crypto.go)
- **Purpose:** Encrypt/decrypt all data
- **Implementation:**
  ```go
  type Key struct {
      Encrypt [32]byte  // AES-256 key
      MAC     [32]byte  // authentication key (used as additional data)
  }

  func DeriveKey(password string, salt []byte) *Key     // Argon2id
  func (k *Key) Encrypt(plaintext []byte) ([]byte, error)  // AES-256-GCM
  func (k *Key) Decrypt(ciphertext []byte) ([]byte, error)  // AES-256-GCM
  ```
- **Format:** `[12-byte nonce][ciphertext + 16-byte auth tag]`

### Packer (internal/repository/packer.go)
- **Purpose:** Bundle multiple blobs into a single pack file
- **Pack format (binary):**
  ```
  [Blob 1 encrypted data][Blob 2 encrypted data]...[Blob N encrypted data][Encrypted Header][4-byte header size]
  ```
- **Header entry (per blob):**
  - Type (1 byte): DataBlob=1 or TreeBlob=2
  - Length (4 bytes): encrypted data size
  - ID (32 bytes): SHA256 blob ID
- **Implementation:**
  ```go
  type Packer struct {
      blobs  []Blob
      buf    *bytes.Buffer
      key    *crypto.Key
  }

  func NewPacker(key *crypto.Key) *Packer
  func (p *Packer) AddBlob(btype BlobType, id ID, data []byte) error
  func (p *Packer) Finalize() ([]byte, error)  // returns complete pack data
  func (p *Packer) Count() int
  ```

### Index (internal/repository/index.go)
- **Purpose:** In-memory lookup: blob ID -> pack location
- **Implementation:**
  ```go
  type Index struct {
      mu      sync.RWMutex
      entries map[BlobHandle]PackedBlob
  }

  func NewIndex() *Index
  func (idx *Index) Store(blob PackedBlob)
  func (idx *Index) Lookup(h BlobHandle) (PackedBlob, bool)
  func (idx *Index) Has(h BlobHandle) bool
  func (idx *Index) Count() int
  func (idx *Index) Marshal() ([]byte, error)       // JSON for persistence
  func UnmarshalIndex(data []byte) (*Index, error)
  ```

### Repository (internal/repository/repository.go)
- **Purpose:** High-level operations combining backend, index, crypto
- **Implementation:**
  ```go
  type Repository struct {
      be     backend.Backend
      key    *crypto.Key
      idx    *Index
      config Config
  }

  func InitRepository(path, password string) (*Repository, error)
  func OpenRepository(path, password string) (*Repository, error)
  func (r *Repository) SaveBlob(ctx context.Context, t BlobType, data []byte) (ID, error)
  func (r *Repository) LoadBlob(ctx context.Context, t BlobType, id ID) ([]byte, error)
  func (r *Repository) SaveSnapshot(ctx context.Context, sn *Snapshot) (ID, error)
  func (r *Repository) LoadSnapshot(ctx context.Context, id ID) (*Snapshot, error)
  func (r *Repository) ListSnapshots(ctx context.Context) ([]*Snapshot, error)
  func (r *Repository) Flush(ctx context.Context) error   // finalize pending packs
  func (r *Repository) SaveIndex(ctx context.Context) error
  func (r *Repository) LoadIndex(ctx context.Context) error
  ```

### Archiver (internal/archiver/archiver.go)
- **Purpose:** Create backups by walking filesystem and saving to repository
- **Implementation:**
  ```go
  type Archiver struct {
      repo *repository.Repository
  }

  func New(repo *repository.Repository) *Archiver
  func (a *Archiver) Backup(ctx context.Context, paths []string) (*Snapshot, error)
  func (a *Archiver) saveFile(ctx context.Context, path string) ([]ID, error)
  func (a *Archiver) saveDir(ctx context.Context, path string) (ID, error)
  ```
- **File saving:**
  1. Read file, split into chunks (fixed-size 1MB or Rabin fingerprint)
  2. Hash each chunk -> ID
  3. Check index: skip if exists (dedup)
  4. Encrypt and save via repo.SaveBlob()
  5. Return list of blob IDs (Node.Content)
- **Dir saving:**
  1. ReadDir, create Node for each entry
  2. Recurse for subdirectories (get Subtree ID)
  3. Recurse for files (get Content IDs)
  4. Create Tree, sort, marshal to JSON
  5. Save as TreeBlob, return tree ID

### Restorer (internal/restorer/restorer.go)
- **Purpose:** Restore files from a snapshot
- **Implementation:**
  ```go
  type Restorer struct {
      repo *repository.Repository
  }

  func New(repo *repository.Repository) *Restorer
  func (r *Restorer) Restore(ctx context.Context, snapshotID ID, targetDir string) error
  func (r *Restorer) restoreTree(ctx context.Context, treeID ID, dir string) error
  func (r *Restorer) restoreFile(ctx context.Context, node *Node, path string) error
  ```

### Chunker (internal/chunker/chunker.go)
- **Purpose:** Split file data into variable or fixed-size chunks
- **Implementation:**
  ```go
  const DefaultChunkSize = 1 << 20  // 1 MB
  const MinChunkSize = 512 << 10    // 512 KB
  const MaxChunkSize = 8 << 20      // 8 MB

  func Chunk(r io.Reader, chunkSize int) ([][]byte, error)
  // For simplicity, use fixed-size chunking
  // Advanced: Rabin fingerprint rolling hash for content-defined boundaries
  ```

## Configuration

- CLI flags (flag or cobra):
  - `-r, --repo PATH` — repository path (required)
  - `--password` or env `RESTIC_PASSWORD` — encryption password
- Repository config stored encrypted in `repo/config`

## Test Cases

### Functional Tests
1. **Init repository:** `restic-mini init -r /tmp/repo` -> creates repo structure (config, keys/, data/, snapshots/, index/)
2. **Backup files:** `restic-mini backup /tmp/testdata -r /tmp/repo` -> creates snapshot with correct file count
3. **List snapshots:** `restic-mini snapshots -r /tmp/repo` -> shows snapshot with timestamp, paths, hostname
4. **Restore backup:** `restic-mini restore LATEST -r /tmp/repo -t /tmp/restored` -> files match originals
5. **Deduplication:** Backup same data twice -> second backup creates no new data blobs
6. **Large file chunking:** Backup 10MB file -> split into multiple chunks
7. **Ls command:** `restic-mini ls SNAPSHOT_ID -r /tmp/repo` -> lists files in snapshot tree

### Edge Cases
1. **Empty directory:** Backup dir with no files -> tree with 0 nodes
2. **Nested directories:** Deep nesting (5+ levels) -> correct tree structure
3. **Binary files:** Non-UTF8 content backed up and restored correctly
4. **Wrong password:** Open repo with wrong password -> error

## Build & Run

### Build
```bash
go build -o restic-mini ./cmd/restic-mini
```

### Run
```bash
# Initialize repository
./restic-mini init -r /tmp/backup-repo
# Enter password when prompted

# Create backup
./restic-mini backup /path/to/data -r /tmp/backup-repo

# List snapshots
./restic-mini snapshots -r /tmp/backup-repo

# List files in snapshot
./restic-mini ls SNAPSHOT_ID -r /tmp/backup-repo

# Restore
./restic-mini restore SNAPSHOT_ID -r /tmp/backup-repo -t /tmp/restored
```

### Test
```bash
go test ./...
```

## Acceptance Criteria

1. `go build` succeeds without errors
2. `init` creates valid repository directory structure
3. `backup` creates encrypted pack files containing file data
4. `backup` creates tree blobs representing directory structure
5. `backup` creates snapshot with correct metadata (time, paths, hostname)
6. Content-addressable storage works (same data -> same blob ID)
7. Deduplication works (second backup of same data creates no new blobs)
8. `restore` recreates files with identical content to originals
9. `restore` preserves directory structure
10. `restore` sets file permissions and modification times
11. Encryption works (pack files are not readable without password)
12. Wrong password produces clear error message
13. `snapshots` lists all snapshots with human-readable output
14. `ls` shows file tree from a snapshot
15. Pack files contain multiple blobs with encrypted header
16. Index persisted and loaded correctly across sessions
