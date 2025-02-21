package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gobuffalo/pop/v5"
	"github.com/gobuffalo/validate/v3"
	"github.com/gofrs/uuid"
	_ "golang.org/x/image/webp" // enable decoding of WEBP images

	"github.com/silinternational/cover-api/api"
	"github.com/silinternational/cover-api/domain"
	"github.com/silinternational/cover-api/storage"
)

const minimumFileLifespan = time.Minute * 5

type FileUploadError struct {
	HttpStatus int
	ErrorCode  api.ErrorKey
	Message    string
}

func (f *FileUploadError) Error() string {
	return fmt.Sprintf("%d: %s ... %s", f.HttpStatus, f.ErrorCode, f.Message)
}

type File struct {
	ID            uuid.UUID `db:"id"`
	URL           string    `db:"url" validate:"required"`
	URLExpiration time.Time `db:"url_expiration"`
	Name          string    `db:"name" validate:"required"`
	Size          int       `db:"size" validate:"required,min=0"`
	ContentType   string    `db:"content_type" validate:"required"`
	Linked        bool      `db:"linked"`
	CreatedByID   uuid.UUID `db:"created_by_id" validate:"required"`

	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`

	Content []byte `db:"-"`
}

// String can be helpful for serializing the model
func (f File) String() string {
	jf, _ := json.Marshal(f)
	return string(jf)
}

// Files is merely for convenience and brevity
type Files []File

// String can be helpful for serializing the model
func (f Files) String() string {
	jf, _ := json.Marshal(f)
	return string(jf)
}

// Validate gets run every time you call a "pop.Validate*" (pop.ValidateAndSave, pop.ValidateAndCreate, pop.ValidateAndUpdate) method.
func (f *File) Validate(tx *pop.Connection) (*validate.Errors, error) {
	return validateModel(f), nil
}

// Store takes a byte slice and stores it into S3 and saves the metadata in the database file table.
func (f *File) Store(tx *pop.Connection) *FileUploadError {
	if len(f.Content) > domain.MaxFileSize {
		e := FileUploadError{
			HttpStatus: http.StatusBadRequest,
			ErrorCode:  api.ErrorStoreFileTooLarge,
			Message:    fmt.Sprintf("file too large (%d bytes), max is %d bytes", len(f.Content), domain.MaxFileSize),
		}
		return &e
	}

	contentType, err := validateContentType(f.Content)
	if err != nil {
		e := FileUploadError{
			HttpStatus: http.StatusBadRequest,
			ErrorCode:  api.ErrorStoreFileBadContentType,
			Message:    err.Error(),
		}
		return &e
	}

	f.ContentType = contentType
	f.removeMetadata()
	f.changeFileExtension()

	f.ID = domain.GetUUID()

	url, err := storage.StoreFile(f.ID.String(), contentType, f.Content)
	if err != nil {
		e := FileUploadError{
			HttpStatus: http.StatusInternalServerError,
			ErrorCode:  api.ErrorUnableToStoreFile,
			Message:    fmt.Sprintf("error %s storing file: %+v", err, f),
		}
		return &e
	}

	f.URL = url.Url
	f.URLExpiration = url.Expiration
	f.Size = len(f.Content)
	if err = f.Create(tx); err != nil {
		e := FileUploadError{
			HttpStatus: http.StatusInternalServerError,
			ErrorCode:  api.ErrorUnableToStoreFile,
			Message:    fmt.Sprintf("error %s creating file: %+v", err, f),
		}
		return &e
	}

	return nil
}

// removeMetadata removes, if possible, all EXIF metadata by re-encoding the image. If the encoding type changes,
// `contentType` will be modified accordingly.
func (f *File) removeMetadata() {
	img, _, err := image.Decode(bytes.NewReader(f.Content))
	if err != nil {
		return
	}
	buf := new(bytes.Buffer)
	switch f.ContentType {
	case "image/jpg":
		if err := jpeg.Encode(buf, img, nil); err == nil {
			f.Content = buf.Bytes()
		}
	case "image/gif":
		if err := gif.Encode(buf, img, nil); err == nil {
			f.Content = buf.Bytes()
		}
	case "image/png":
		if err := png.Encode(buf, img); err == nil {
			f.Content = buf.Bytes()
		}
	case "image/webp":
		if err := png.Encode(buf, img); err == nil {
			f.Content = buf.Bytes()
			f.ContentType = "image/png"
		}
	}
}

// changeFileExtension attempts to make the file extension match the given content type
func (f *File) changeFileExtension() {
	ext, err := mime.ExtensionsByType(f.ContentType)
	if err != nil || len(ext) < 1 {
		return
	}
	f.Name = strings.TrimSuffix(f.Name, filepath.Ext(f.Name)) + ext[0]
}

// Find locates a file by ID and returns the result, including a valid URL.
// None of the struct members of f are used as input, but are updated if the function is successful.
func (f *File) Find(tx *pop.Connection, id uuid.UUID) error {
	var file File
	if err := tx.Find(&file, id); err != nil {
		return err
	}
	*f = file
	return nil
}

// RefreshURL ensures the file URL is good for at least a few minutes
func (f *File) RefreshURL(tx *pop.Connection) error {
	if f.URLExpiration.After(time.Now().Add(minimumFileLifespan)) {
		return nil
	}

	newURL, err := storage.GetFileURL(f.ID.String())
	if err != nil {
		return err
	}
	f.URL = newURL.Url
	f.URLExpiration = newURL.Expiration
	if err = f.Update(tx); err != nil {
		return err
	}
	return nil
}

func validateContentType(content []byte) (string, error) {
	detectedType := http.DetectContentType(content)
	if domain.IsStringInSlice(detectedType, domain.AllowedFileUploadTypes) {
		return detectedType, nil
	}
	return "", fmt.Errorf("invalid file type %s", detectedType)
}

// Create stores the File data as a new record in the database.
func (f *File) Create(tx *pop.Connection) error {
	return create(tx, f)
}

// Update writes the File data to an existing database record.
func (f *File) Update(tx *pop.Connection) error {
	return update(tx, f)
}

// DeleteUnlinked removes all files that are no longer linked to any database records
func (f *Files) DeleteUnlinked(tx *pop.Connection) error {
	var files Files
	if err := tx.Select("id", "uuid").
		Where("linked = FALSE AND updated_at < ?", time.Now().Add(-4*domain.DurationWeek)).
		All(&files); err != nil {
		return err
	}
	domain.Logger.Printf("unlinked files: %d", len(files))
	if len(files) > domain.Env.MaxFileDelete {
		return fmt.Errorf("attempted to delete too many files, MaxFileDelete=%d", domain.Env.MaxFileDelete)
	}
	if len(files) == 0 {
		return nil
	}

	nRemovedFromDB := 0
	nRemovedFromS3 := 0
	for _, file := range files {
		if err := storage.RemoveFile(file.ID.String()); err != nil {
			domain.ErrLogger.Printf("error removing from S3, id='%s', %s", file.ID.String(), err)
			continue
		}
		nRemovedFromS3++

		f := file
		if err := tx.Destroy(&f); err != nil {
			domain.ErrLogger.Printf("file %d destroy error, %s", file.ID, err)
			continue
		}
		nRemovedFromDB++
	}

	if nRemovedFromDB < len(files) || nRemovedFromS3 < len(files) {
		domain.ErrLogger.Printf("not all unlinked files were removed")
	}
	domain.Logger.Printf("removed %d from S3, %d from file table", nRemovedFromS3, nRemovedFromDB)
	return nil
}

// SetLinked marks the file as linked. If already linked, return an error since it may be attempting to link a file to
// multiple records.
// The File struct need not be hydrated; only the ID is needed.
func (f *File) SetLinked(tx *pop.Connection) error {
	if err := tx.Reload(f); err != nil {
		panic(fmt.Sprintf("failed to load file for setting linked flag, %s", err))
	}
	if f.Linked {
		err := fmt.Errorf("cannot link file, it is already linked")
		return api.NewAppError(err, api.ErrorFileAlreadyLinked, api.CategoryUser)
	}
	f.Linked = true
	if err := tx.UpdateColumns(f, "linked", "updated_at"); err != nil {
		return appErrorFromDB(err, api.ErrorUpdateFailure)
	}
	return nil
}

// ClearLinked marks the file as unlinked. The struct need not be hydrated; only the ID is needed.
func (f *File) ClearLinked(tx *pop.Connection) error {
	f.Linked = false
	return tx.UpdateColumns(f, "linked", "updated_at")
}

// FindByIDs finds all Files associated with the given IDs and loads them from the database
func (f *Files) FindByIDs(tx *pop.Connection, ids []int) error {
	return tx.Where("id in (?)", ids).All(f)
}

// ConvertToAPI converts a models.File to an api.File
func (f *File) ConvertToAPI(tx *pop.Connection) api.File {
	if err := f.RefreshURL(tx); err != nil {
		panic(err.Error())
	}
	return api.File{
		ID:            f.ID,
		URL:           f.URL,
		URLExpiration: f.URLExpiration,
		Name:          f.Name,
		Size:          f.Size,
		ContentType:   f.ContentType,
		CreatedByID:   f.CreatedByID,
	}
}
