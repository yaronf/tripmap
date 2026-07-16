package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3 implements Store against a versioned itineraries bucket.
type S3 struct {
	Client *s3.Client
	Bucket string
}

func (s *S3) ListTripIDs(ctx context.Context) ([]string, error) {
	var ids []string
	seen := map[string]bool{}
	paginator := s3.NewListObjectsV2Paginator(s.Client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.Bucket),
		Prefix: aws.String("trips/"),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			if id, ok := tripIDFromYAMLKey(aws.ToString(obj.Key)); ok && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	return ids, nil
}

func (s *S3) Exists(ctx context.Context, id string) (bool, error) {
	_, err := s.Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(yamlKey(id)),
	})
	if err != nil {
		// treat not found as false
		return false, nil
	}
	return true, nil
}

func (s *S3) GetYAML(ctx context.Context, id string) (YAMLObject, error) {
	out, err := s.Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(yamlKey(id)),
	})
	if err != nil {
		return YAMLObject{}, fmt.Errorf("get yaml %s: %w", id, err)
	}
	defer out.Body.Close()
	body, err := io.ReadAll(out.Body)
	if err != nil {
		return YAMLObject{}, err
	}
	return YAMLObject{Body: body, VersionID: aws.ToString(out.VersionId)}, nil
}

func (s *S3) GetYAMLVersion(ctx context.Context, id, versionID string) (YAMLObject, error) {
	out, err := s.Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket:    aws.String(s.Bucket),
		Key:       aws.String(yamlKey(id)),
		VersionId: aws.String(versionID),
	})
	if err != nil {
		return YAMLObject{}, fmt.Errorf("get yaml %s@%s: %w", id, versionID, err)
	}
	defer out.Body.Close()
	body, err := io.ReadAll(out.Body)
	if err != nil {
		return YAMLObject{}, err
	}
	return YAMLObject{Body: body, VersionID: aws.ToString(out.VersionId)}, nil
}

func (s *S3) PutYAML(ctx context.Context, id string, body []byte) (string, error) {
	out, err := s.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.Bucket),
		Key:         aws.String(yamlKey(id)),
		Body:        bytes.NewReader(body),
		ContentType: aws.String("application/yaml"),
	})
	if err != nil {
		return "", err
	}
	return aws.ToString(out.VersionId), nil
}

func (s *S3) GetMeta(ctx context.Context, id string) (Meta, error) {
	out, err := s.Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(metaKey(id)),
	})
	if err != nil {
		return Meta{}, fmt.Errorf("get meta %s: %w", id, err)
	}
	defer out.Body.Close()
	var m Meta
	if err := json.NewDecoder(out.Body).Decode(&m); err != nil {
		return Meta{}, err
	}
	return m, nil
}

func (s *S3) PutMeta(ctx context.Context, id string, m Meta) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	_, err = s.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.Bucket),
		Key:         aws.String(metaKey(id)),
		Body:        bytes.NewReader(append(b, '\n')),
		ContentType: aws.String("application/json"),
	})
	return err
}

func (s *S3) ListVersions(ctx context.Context, id string) ([]VersionInfo, error) {
	out, err := s.Client.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{
		Bucket: aws.String(s.Bucket),
		Prefix: aws.String(yamlKey(id)),
	})
	if err != nil {
		return nil, err
	}
	var vers []VersionInfo
	for _, v := range out.Versions {
		if aws.ToString(v.Key) != yamlKey(id) {
			continue
		}
		vers = append(vers, VersionInfo{
			VersionID:    aws.ToString(v.VersionId),
			LastModified: aws.ToTime(v.LastModified),
			IsLatest:     aws.ToBool(v.IsLatest),
		})
	}
	return vers, nil
}

func (s *S3) GetIdempotency(ctx context.Context, key string) ([]byte, bool, error) {
	out, err := s.Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(idemKey(key)),
	})
	if err != nil {
		return nil, false, nil
	}
	defer out.Body.Close()
	b, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

func (s *S3) PutIdempotency(ctx context.Context, key string, body []byte) error {
	_, err := s.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.Bucket),
		Key:         aws.String(idemKey(key)),
		Body:        bytes.NewReader(body),
		ContentType: aws.String("application/json"),
	})
	return err
}

func (s *S3) UploadBundle(ctx context.Context, id string, root string) error {
	prefix := bundlePrefix(id)
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		ct := mime.TypeByExtension(filepath.Ext(rel))
		if ct == "" {
			ct = "application/octet-stream"
		}
		if strings.HasSuffix(rel, ".json") {
			ct = "application/json"
		}
		if strings.HasSuffix(rel, ".html") {
			ct = "text/html; charset=utf-8"
		}
		if strings.HasSuffix(rel, ".js") {
			ct = "text/javascript; charset=utf-8"
		}
		if strings.HasSuffix(rel, ".css") {
			ct = "text/css; charset=utf-8"
		}
		_, err = s.Client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:      aws.String(s.Bucket),
			Key:         aws.String(prefix + rel),
			Body:        bytes.NewReader(b),
			ContentType: aws.String(ct),
		})
		return err
	})
}