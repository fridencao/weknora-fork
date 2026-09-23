package docparser

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/docreader/proto"
	"github.com/Tencent/WeKnora/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// captureClient 记录发往 docreader 的 ReadRequest，用于断言字段映射。
type captureClient struct{ got *proto.ReadRequest }

func (c *captureClient) Read(
	_ context.Context, in *proto.ReadRequest, _ ...grpc.CallOption,
) (*proto.ReadResponse, error) {
	c.got = in
	return &proto.ReadResponse{MarkdownContent: "md"}, nil
}

func (c *captureClient) ReadStream(
	_ context.Context, in *proto.ReadRequest, _ ...grpc.CallOption,
) (grpc.ServerStreamingClient[proto.ReadStreamResponse], error) {
	c.got = in
	// 让 Read 回落到 unary Read，从而走到上面的捕获点。
	return nil, status.Error(codes.Unimplemented, "not implemented in test")
}

func (c *captureClient) ListEngines(
	_ context.Context, _ *proto.ListEnginesRequest, _ ...grpc.CallOption,
) (*proto.ListEnginesResponse, error) {
	return &proto.ListEnginesResponse{}, nil
}

// TestReadRequestCarriesDocID 锁死 types.ReadRequest.DocID → proto.ReadRequest.doc_id
// 的映射。这条链路一旦静默断掉，StarKB 的契约目录名就会与 WeKnora 知识 ID 错位，
// 图谱回填会以「契约包缺失」全部失败——正是本次修复的 bug。
func TestReadRequestCarriesDocID(t *testing.T) {
	const kid = "d6b20058-4a1e-4b3f-9c2e-0f1a2b3c4d5e"

	c := &captureClient{}
	p := &GRPCDocumentReader{client: c}

	if _, err := p.Read(context.Background(), &types.ReadRequest{
		FileName: "国际先进银行零售业务战略研究.pdf",
		FileType: "pdf",
		DocID:    kid,
	}); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if c.got == nil {
		t.Fatal("未捕获到 ReadRequest")
	}
	if c.got.DocId != kid {
		t.Fatalf("DocId 未透传：got %q, want %q", c.got.DocId, kid)
	}
	if c.got.FileName != "国际先进银行零售业务战略研究.pdf" {
		t.Fatalf("FileName 异常：got %q", c.got.FileName)
	}
}

// TestReadRequestOmitsEmptyDocID 保证未提供 DocID 时不发该字段（旧调用方行为不变）。
func TestReadRequestOmitsEmptyDocID(t *testing.T) {
	c := &captureClient{}
	p := &GRPCDocumentReader{client: c}

	if _, err := p.Read(context.Background(), &types.ReadRequest{
		FileName: "a.pdf", FileType: "pdf",
	}); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if c.got == nil {
		t.Fatal("未捕获到 ReadRequest")
	}
	if c.got.DocId != "" {
		t.Fatalf("未传 DocID 时不应有值：got %q", c.got.DocId)
	}
}
