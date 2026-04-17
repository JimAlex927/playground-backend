// Package minio 封装 MinIO 基础能力，供其他 infra 组件复用。
//
// 提供两种客户端：
//   - minio-go Client：用于桶管理、presigned URL、对象列举等 MinIO 原生操作
//   - AWS SDK v2 S3 Client：用于 tusd s3store（TUS 分片上传协议要求）
//
// 两者使用同一份配置（Config），指向同一个 MinIO 实例。
package minio

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	miniogo "github.com/minio/minio-go/v7"
	miniocreds "github.com/minio/minio-go/v7/pkg/credentials"
)

// Config 是 MinIO 连接配置，由 config 层读取环境变量后填充。
type Config struct {
	Endpoint        string // e.g. "localhost:9000"
	AccessKeyID     string
	SecretAccessKey string
	UseSSL          bool
	BucketName      string
	Region          string // 默认 "us-east-1"（MinIO 忽略此值，但 AWS SDK 需要）
}

func (c Config) region() string {
	if c.Region == "" {
		return "us-east-1"
	}
	return c.Region
}

func (c Config) scheme() string {
	if c.UseSSL {
		return "https"
	}
	return "http"
}

// NewMinioClient 创建 minio-go 原生客户端。
//
// 适用场景：
//   - 检查 / 创建 bucket（EnsureBucket）
//   - 生成 presigned 下载 URL（供前端直接下载文件）
//   - 列举对象、删除对象等通用存储操作
func NewMinioClient(cfg Config) (*miniogo.Client, error) {
	client, err := miniogo.New(cfg.Endpoint, &miniogo.Options{
		Creds:  miniocreds.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}
	return client, nil
}

// NewS3Client 创建 AWS SDK v2 的 S3 客户端，指向 MinIO 实例。
//
// 适用场景：
//   - 给 tusd 的 s3store 使用（tusd 依赖 AWS SDK v2 的 S3 接口）
//   - UsePathStyle = true 是 MinIO 必须的配置项
func NewS3Client(cfg Config) *s3.Client {
	return s3.New(s3.Options{
		Region:       cfg.region(),
		BaseEndpoint: aws.String(fmt.Sprintf("%s://%s", cfg.scheme(), cfg.Endpoint)),
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		UsePathStyle: true, // MinIO 要求 path-style，不支持 virtual-hosted-style
	})
}

// EnsureBucket 检查 bucket 是否存在，不存在则自动创建。
// 建议在服务启动时调用一次，确保环境就绪。
func EnsureBucket(ctx context.Context, client *miniogo.Client, cfg Config) error {
	exists, err := client.BucketExists(ctx, cfg.BucketName)
	if err != nil {
		return fmt.Errorf("check bucket %q: %w", cfg.BucketName, err)
	}
	if exists {
		return nil
	}
	if err := client.MakeBucket(ctx, cfg.BucketName, miniogo.MakeBucketOptions{
		Region: cfg.region(),
	}); err != nil {
		return fmt.Errorf("create bucket %q: %w", cfg.BucketName, err)
	}
	return nil
}
