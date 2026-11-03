package provider

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestAskGridHiRes(t *testing.T) {
	if os.Getenv("GEOM_MANUAL") == "" {
		t.Skip("manual only")
	}
	img, err := os.ReadFile("/tmp/g36_hi.jpg")
	if err != nil {
		t.Fatal(err)
	}
	prov := NewZhipuProvider(ZhipuConfig{APIKey: os.Getenv("ZHIPU_API_KEY")})
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()
	prompt := "这张图里有一个浅灰色的小正方形网格（方格纸）。请数清楚后只回答一行：列=? 行=?（列=横向格子数，行=纵向格子数）。不要解释。"
	for i := 0; i < 2; i++ {
		out, qerr := prov.dash.chat(ctx, prompt, img)
		if qerr != nil {
			t.Fatalf("chat: %v", qerr)
		}
		t.Logf("hi-res #%d: %s", i+1, out)
	}
}
