package filebrowser

import (
	"context"
	stderrors "errors"
	"mime"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"k8s.io/utils/ptr"
	"xiaoshiai.cn/common/errors"
	libs3 "xiaoshiai.cn/common/s3"
)

type S3WebBrowser struct {
	S3       libs3.Client
	Bucket   string
	PartSize int64 // must set same as frontend part size
}

// OpenMultiPartUpload implements WebBrowser.
func (s *S3WebBrowser) OpenMultiPartUpload(ctx context.Context, path string) (string, error) {
	result, err := s.S3.Client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{Bucket: &s.Bucket, Key: &path})
	if err != nil {
		return "", err
	}
	return *result.UploadId, nil
}

// UploadPart implements WebBrowser.
func (s *S3WebBrowser) UploadPart(ctx context.Context, uploadID string, offset, total int64, content FileContent) error {
	s3content := s3.UploadPartInput{
		Bucket:        &s.Bucket,
		Key:           &content.Name,
		UploadId:      &uploadID,
		PartNumber:    ptr.To(int32(offset/s.PartSize + 1)),
		ContentLength: ptr.To(content.ContentLength),
	}
	output, err := s.S3.Client.UploadPart(ctx, &s3content)
	if err != nil {
		return err
	}
	_ = output
	return nil
}

// CancelMultiPartUpload implements WebBrowser.
func (s *S3WebBrowser) CancelMultiPartUpload(ctx context.Context, uploadID string) error {
	_, err := s.S3.Client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{Bucket: &s.Bucket, Key: &uploadID})
	return err
}

// CompleteMultiPartUpload implements WebBrowser.
func (s *S3WebBrowser) CompleteMultiPartUpload(ctx context.Context, uploadID string) error {
	_, err := s.S3.Client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{Bucket: &s.Bucket, Key: &uploadID})
	return err
}

// CopyFile implements WebBrowser.
func (s *S3WebBrowser) CopyFile(ctx context.Context, src string, dest string) error {
	s3copy := s3.CopyObjectInput{
		Bucket:     &s.Bucket,
		CopySource: &src,
		Key:        &dest,
	}
	_, err := s.S3.Client.CopyObject(ctx, &s3copy)
	return err
}

// DeleteFile implements WebBrowser.
func (s *S3WebBrowser) DeleteFile(ctx context.Context, path string, all bool) error {
	if all {
		if !strings.HasSuffix(path, "/") {
			path += "/"
		}
		var deleteobjects []types.ObjectIdentifier
		listinput := &s3.ListObjectsV2Input{Bucket: &s.Bucket, Prefix: &path}
		for {
			listoutput, err := s.S3.Client.ListObjectsV2(ctx, listinput)
			if err != nil {
				return err
			}
			if len(listoutput.Contents) == 0 {
				break
			}
			if aws.ToBool(listoutput.IsTruncated) {
				listinput.ContinuationToken = listoutput.NextContinuationToken
			} else {
				listinput.ContinuationToken = nil
			}
			for _, obj := range listoutput.Contents {
				deleteobjects = append(deleteobjects, types.ObjectIdentifier{Key: obj.Key})
			}
		}
		if len(deleteobjects) > 0 {
			for i := 0; i < len(deleteobjects); i += 1000 {
				deleteinput := &s3.DeleteObjectsInput{
					Bucket: &s.Bucket,
					Delete: &types.Delete{
						Objects: deleteobjects[i:min(i+1000, len(deleteobjects))],
						Quiet:   aws.Bool(false),
					},
				}
				if _, err := s.S3.Client.DeleteObjects(ctx, deleteinput); err != nil {
					return err
				}
			}
		}
		return nil
	} else {
		_, err := s.S3.Client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &s.Bucket, Key: &path})
		return err
	}
}

// DownloadFile implements WebBrowser.
func (s *S3WebBrowser) DownloadFile(ctx context.Context, path string, options DownloadFileOptions) (*FileContent, error) {
	input := &s3.GetObjectInput{
		Bucket: &s.Bucket, Key: &path,
	}
	if options.Range != "" {
		input.Range = &options.Range
	}
	if options.IfMatch != "" {
		input.IfMatch = &options.IfMatch
	}
	if options.IfNoneMatch != "" {
		input.IfNoneMatch = &options.IfNoneMatch
	}
	output, err := s.S3.Client.GetObject(ctx, input)
	if err != nil {
		return nil, err
	}
	return &FileContent{
		Name:          path,
		Content:       output.Body,
		ContentType:   aws.ToString(output.ContentType),
		ContentLength: aws.ToInt64(output.ContentLength),
		ContentRange:  aws.ToString(output.ContentRange),
		LastModified:  aws.ToTime(output.LastModified),
		Etag:          aws.ToString(output.ETag),
	}, nil
}

// LinkFile implements WebBrowser.
func (s *S3WebBrowser) LinkFile(ctx context.Context, src string, dest string) error {
	return errors.NewUnsupported("s3 does not support link")
}

// MoveFile implements WebBrowser.
func (s *S3WebBrowser) MoveFile(ctx context.Context, src string, dest string) error {
	return errors.NewUnsupported("s3 does not support move, use copy and delete instead")
}

// StateFile implements WebBrowser.
func (s *S3WebBrowser) StateFile(ctx context.Context, path string, options StateFileOptions) (*TreeItem, error) {
	stat, err := s.S3.Client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &s.Bucket, Key: &path})
	if err != nil {
		if GetS3ErrStatusCode(err) == 404 {
			// try dir list
			return s.listDir(ctx, path, options)
		}
		return nil, err
	}
	return &TreeItem{
		Name:        path,
		Type:        TreeItemTypeFile,
		Size:        aws.ToInt64(stat.ContentLength),
		ContentType: aws.ToString(stat.ContentType),
		Permission:  "rw-rw-rw-",
		ModTime:     aws.ToTime(stat.LastModified),
		Attributes:  stat.Metadata,
	}, nil
}

func (s *S3WebBrowser) listDir(ctx context.Context, path string, options StateFileOptions) (*TreeItem, error) {
	input := &s3.ListObjectsV2Input{
		Bucket: &s.Bucket,
		Prefix: &path,
	}
	if options.Limit > 0 {
		input.MaxKeys = aws.Int32(int32(options.Limit))
	}
	if options.Continue != "" {
		input.ContinuationToken = &options.Continue
	}
	var items []TreeItem
	for {
		s3output, err := s.S3.Client.ListObjectsV2(ctx, input)
		if err != nil {
			return nil, err
		}
		if len(s3output.Contents) == 0 {
			break
		}
		if aws.ToBool(s3output.IsTruncated) {
			input.ContinuationToken = s3output.NextContinuationToken
		} else {
			input.ContinuationToken = nil
		}
		for _, obj := range s3output.Contents {
			items = append(items, TypesObjectToTreeItem(obj))
		}
	}
	diritem := &TreeItem{
		Name:     path,
		Type:     TreeItemTypeDir,
		Childern: items,
		Continue: aws.ToString(input.ContinuationToken),
	}
	return diritem, nil
}

func TypesObjectToTreeItem(obj types.Object) TreeItem {
	return TreeItem{
		Name:        aws.ToString(obj.Key),
		Type:        TreeItemTypeFile,
		Size:        aws.ToInt64(obj.Size),
		ContentType: mime.TypeByExtension(path.Ext(aws.ToString(obj.Key))),
		Permission:  "rw-rw-rw-",
		Etag:        aws.ToString(obj.ETag),
		ModTime:     aws.ToTime(obj.LastModified),
	}
}

func GetS3ErrStatusCode(err error) int {
	var apie *smithyhttp.ResponseError
	if stderrors.As(err, &apie) {
		return apie.HTTPStatusCode()
	}
	return 0
}

// UploadFile implements WebBrowser.
func (s *S3WebBrowser) UploadFile(ctx context.Context, path string, content FileContent) error {
	input := &s3.PutObjectInput{
		Bucket: &s.Bucket,
		Key:    &path,
		Body:   content.Content,
	}
	if content.ContentType != "" {
		input.ContentType = &content.ContentType
	}
	if content.ContentLength > 0 {
		input.ContentLength = &content.ContentLength
	}
	_, err := s.S3.Client.PutObject(ctx, input)
	return err
}

var _ WebBrowser = &S3WebBrowser{}
