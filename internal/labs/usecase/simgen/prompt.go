package simgen

import (
	"encoding/json"
	"fmt"

	"github.com/devforge/be/internal/labs/domain"
	"github.com/devforge/be/internal/labs/usecase/sim"
)

// The model writes an array of steps, not the map the engine stores. Structured
// outputs require `additionalProperties: false` on every object, which a map
// with author-invented keys cannot express — so the shape on the wire has names
// as a field and this file converts. Nothing is lost: the conversion is where a
// duplicate step name would be caught, and a map cannot hold one anyway.
type genStep struct {
	Name      string `json:"name"`
	Seconds   int    `json:"seconds"`
	Cacheable string `json:"cacheable"`
	Produces  string `json:"produces"`
	Consumes  string `json:"consumes"`
	Flaky     int    `json:"flaky"`
}

type genOutput struct {
	Notes               string                   `json:"notes"`
	RunnerCount         int                      `json:"runner_count"`
	CacheRestoreSeconds int                      `json:"cache_restore_seconds"`
	Steps               []genStep                `json:"steps"`
	Examples            []domain.ScenarioExample `json:"examples"`
}

func decode(raw json.RawMessage) (*domain.Scenario, string, error) {
	var out genOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, "", fmt.Errorf("kết quả không phải JSON đọc được: %w", err)
	}

	sc := &domain.Scenario{
		Version:             1,
		RunnerCount:         out.RunnerCount,
		CacheRestoreSeconds: out.CacheRestoreSeconds,
		Catalog:             make(map[string]domain.ScenarioStep, len(out.Steps)),
		Examples:            out.Examples,
	}
	for _, st := range out.Steps {
		if st.Name == "" {
			return nil, "", fmt.Errorf("có một step không có tên")
		}
		if _, dup := sc.Catalog[st.Name]; dup {
			return nil, "", fmt.Errorf("step %q khai hai lần", st.Name)
		}
		sc.Catalog[st.Name] = domain.ScenarioStep{
			Seconds:   st.Seconds,
			Cacheable: st.Cacheable,
			Produces:  st.Produces,
			Consumes:  st.Consumes,
			Flaky:     st.Flaky,
		}
	}
	return sc, out.Notes, nil
}

// outputSchema constrains generation rather than validating after it: the API
// will not emit anything that fails this, which is why there is no parse-retry
// layer anywhere in this package.
//
// The numeric limits are in the descriptions, not in the schema. Structured
// outputs do not support `minimum`/`maximum`, and duplicating them here would
// create a second copy of bounds `sim.CheckScenario` already owns — a copy that
// would silently drift the first time one of them changes.
func outputSchema() map[string]any {
	str := func(desc string) map[string]any {
		return map[string]any{"type": "string", "description": desc}
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required": []string{
			"notes", "runner_count", "cache_restore_seconds", "steps", "examples",
		},
		"properties": map[string]any{
			"notes": str("Một hoặc hai câu tiếng Việt nói bộ step này mô phỏng cái gì " +
				"và bài học rút ra khi xếp job khác nhau. Hiện cho người dùng đọc."),
			"runner_count": map[string]any{
				"type": "integer",
				"description": fmt.Sprintf(
					"Số job chạy song song tối đa. Từ 1 đến %d.", sim.MaxScenarioRunners),
			},
			"cache_restore_seconds": map[string]any{
				"type":        "integer",
				"description": "Số giây một step tốn khi lấy từ cache thay vì làm lại. Thường 5-15.",
			},
			"steps": map[string]any{
				"type": "array",
				"description": fmt.Sprintf(
					"Bộ step pipeline được phép dùng. Từ 4 đến %d step.", sim.MaxScenarioSteps),
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required": []string{
						"name", "seconds", "cacheable", "produces", "consumes", "flaky",
					},
					"properties": map[string]any{
						"name": str("Tên step, chữ thường và gạch ngang, ví dụ \"npm-ci\", " +
							"\"go-test\", \"docker-build\"."),
						"seconds": map[string]any{
							"type":        "integer",
							"description": "Step này mất bao lâu, tính bằng giây. Ước lượng thực tế.",
						},
						"cacheable": str("Tên khoá cache step này lưu lại, ví dụ \"node_modules\". " +
							"Chuỗi rỗng nếu step không để lại gì đáng cache."),
						"produces": str("Tên artifact step này tạo ra, ví dụ \"dist\". " +
							"Chuỗi rỗng nếu không tạo gì."),
						"consumes": str("Tên artifact step này cần có sẵn. Chuỗi rỗng nếu không cần gì. " +
							"Artifact phải do một step khác trong danh sách này tạo ra."),
						"flaky": map[string]any{
							"type": "integer",
							"description": "Phần trăm số lượt step này hỏng ngẫu nhiên, 0 đến 100. " +
								"Để 0 trừ khi đang cố tình dạy về flaky test.",
						},
					},
				},
			},
			"examples": map[string]any{
				"type": "array",
				"description": "Từ 2 đến 4 pipeline mẫu bấm-là-chạy, xếp từ cách chậm tới cách nhanh, " +
					"để người học so hai con số.",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"title", "note", "pipeline"},
					"properties": map[string]any{
						"title": str("Nhãn ngắn trên nút, ví dụ \"① Nối tiếp\"."),
						"note":  str("Một dòng nói cách xếp này tốn bao nhiêu và vì sao."),
						"pipeline": str("Nội dung file YAML, dùng \\n xuống dòng. " +
							"Chỉ ba khoá: jobs, needs, steps, cache."),
					},
				},
			},
		},
	}
}

// workedExample is one complete, runnable scenario, in exactly the shape the
// model must produce. It is the single strongest signal in the whole prompt:
// rules describe the engine, an example describes the *answer* — naming style,
// the magnitude of the seconds, how a note is phrased, how the examples build a
// comparison. Without one the model has never seen what "good" looks like.
//
// It is the Node.js CI scenario the frontend ships, which is authored content
// somebody wrote on purpose, not something invented for a prompt.
//
// `TestWorkedExampleIsRunnable` puts this through `decode` and `verify` — the
// same path the model's own output takes. An example that stops running is an
// example teaching the model to produce something the engine rejects, and that
// failure would otherwise be invisible until somebody read the generated output
// closely.
const workedExample = `{
  "notes": "Bộ step của một dự án Node: cài, soi, test, build, đóng image. Gộp hết vào một job thì mọi việc chờ nhau; tách ra thì hai runner làm song song.",
  "runner_count": 2,
  "cache_restore_seconds": 10,
  "steps": [
    {"name": "checkout", "seconds": 5, "cacheable": "", "produces": "", "consumes": "", "flaky": 0},
    {"name": "npm-ci", "seconds": 90, "cacheable": "node_modules", "produces": "", "consumes": "", "flaky": 0},
    {"name": "lint", "seconds": 25, "cacheable": "", "produces": "", "consumes": "", "flaky": 0},
    {"name": "npm-test", "seconds": 120, "cacheable": "", "produces": "", "consumes": "", "flaky": 0},
    {"name": "npm-build", "seconds": 60, "cacheable": "", "produces": "dist", "consumes": "", "flaky": 0},
    {"name": "docker-build", "seconds": 180, "cacheable": "", "produces": "", "consumes": "dist", "flaky": 0}
  ],
  "examples": [
    {
      "title": "\u2460 N\u1ed1i ti\u1ebfp",
      "note": "m\u1ed9t job l\u00e0m h\u1ebft \u2014 m\u1ecdi vi\u1ec7c ch\u1edd nhau",
      "pipeline": "jobs:\n  ci:\n    steps: [checkout, npm-ci, npm-test, npm-build, docker-build]\n"
    },
    {
      "title": "\u2461 Song song",
      "note": "t\u00e1ch hai job, b\u1ecf needs \u2014 2 runner c\u00f9ng l\u00e0m",
      "pipeline": "jobs:\n  build:\n    steps: [checkout, npm-ci, npm-build]\n  test:\n    steps: [checkout, npm-ci, npm-test]\n"
    },
    {
      "title": "\u2462 Th\u00eam cache",
      "note": "nh\u01b0 \u2461 nh\u01b0ng khai cache \u2014 b\u1ea5m Ch\u1ea1y HAI l\u01b0\u1ee3t",
      "pipeline": "jobs:\n  build:\n    steps: [checkout, npm-ci, npm-build]\n    cache: [node_modules]\n  test:\n    steps: [checkout, npm-ci, npm-test]\n    cache: [node_modules]\n"
    }
  ]
}`

// systemPrompt is a constant, byte for byte, on every request — that is what
// makes the cache breakpoint on it worth having. Nothing per-user or per-time
// may be added here: either would turn every read into a fresh write.
const systemPrompt = `Bạn viết kịch bản cho một engine mô phỏng pipeline CI/CD dùng để dạy DevOps.

Engine này KHÔNG chạy lệnh thật. Nó chỉ tính lịch: job nào chạy lúc nào, trên
runner nào, mất bao lâu. Việc của bạn là chọn bộ step và giá của từng step sao
cho người học nhìn ra được vì sao xếp job kiểu này nhanh hơn kiểu kia.

# Toàn bộ luật của engine

1. Job không có "needs" thì chạy ngay. Số job chạy cùng lúc bị chặn bởi
   runner_count; job thừa phải xếp hàng đợi runner rảnh.
2. "needs" là job này đợi job kia xong mới bắt đầu.
3. Step trong cùng một job chạy tuần tự, theo đúng thứ tự viết.
4. Một step có "consumes" chỉ chạy được nếu artifact đó đã có: hoặc do một step
   trước đó TRONG CÙNG JOB tạo ra, hoặc do một job nằm trong chuỗi "needs" của
   nó tạo ra. Job chạy song song ở nhánh khác không tính — đĩa của runner đó
   không phải đĩa này.
5. Cache chỉ ấm SANG LƯỢT SAU. Lượt đang chạy vẫn trả đủ giá gốc. Job phải tự
   khai "cache: [khoá]" mới được giảm; quên khai là không giảm.
6. Step hỏng thì job dừng ngay tại đó, các job phụ thuộc nó thành skipped.

# Cú pháp pipeline, hết ba khoá

jobs:
  build:
    steps: [checkout, npm-ci, npm-build]
    cache: [node_modules]
  deploy:
    needs: [build]
    steps: [checkout, docker-build]

# Làm cho ra một bài học

Bộ step phải cho phép ít nhất một trong mấy phép so sánh sau, và examples phải
làm nó hiện ra thành hai con số khác nhau:

- Gộp hết vào một job (chậm) so với tách ra chạy song song (nhanh).
- Có khai cache so với quên khai cache.
- Một dòng "needs" thừa biến hai job song song thành nối tiếp.
- Nhiều job hơn số runner: tách thêm không nhanh hơn nữa.

Muốn thế thì cần vài step ĐẮT (60 giây trở lên) — pipeline toàn step 5 giây thì
xếp kiểu nào cũng như nhau và không dạy được gì.

# Quy mô

**5 đến 8 step.** Giới hạn cứng là 64, nhưng đó là trần chứ không phải mục tiêu:
biểu đồ 20 job không ai đọc nổi, và bài học tan ra thành một mớ thanh ngang.
Người ta hỏi "hệ thống chịu tải hàng triệu user" thì thứ cần mô phỏng vẫn là
*pipeline* của hệ thống đó, không phải kiến trúc của nó.

"runner_count" từ 2 đến 4 là hợp lý. Để 1 khi muốn dạy đúng chuyện "song song
thuộc về đội máy, không thuộc về file bạn đang sửa".

# Mốc thời gian tham chiếu

Để số giây nhất quán, không phải mỗi lần bịa một kiểu:

- lấy code: 5s
- cài thư viện (npm ci, go mod download, pip install): 40-90s
- lint / vet / format check: 15-30s
- unit test: 60-150s
- build / compile: 40-120s
- đóng docker image: 120-240s
- integration test có DB: 150-300s
- e2e trên trình duyệt: 180-300s
- deploy / apply: 60-200s

# Ràng buộc cứng

- Mọi step dùng trong examples phải có trong danh sách steps.
- "consumes" phải trỏ tới artifact có một step khác "produces" ra.
- examples viết YAML hợp lệ, thụt lề bằng khoảng trắng, không dùng tab.

# Lượt sau: sửa, không dựng lại

Người dùng gửi thêm một câu là họ đang **sửa** kịch bản vừa rồi. Trả về kịch bản
đầy đủ, nhưng **giữ nguyên mọi thứ họ không yêu cầu đổi** — cùng tên step, cùng
số giây, cùng examples. "Đổi runner thành 4" nghĩa là đổi đúng một số, không phải
dựng lại một bộ step khác.

Nhận được câu báo lỗi từ engine thì sửa đúng chỗ bị báo, giữ nguyên phần còn lại.

# Yêu cầu lạc đề

Câu hỏi không nói về pipeline CI/CD thì dựng bộ step gần nhất còn có nghĩa cho
công nghệ được nhắc tới, và nói thẳng ở trường notes rằng bạn đã hiểu theo hướng
nào.

# Một kịch bản hoàn chỉnh, đúng dạng phải trả về

` + workedExample + `

Trả lời bằng tiếng Việt ở notes, note và title.`
