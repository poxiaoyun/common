# Validation

一个轻量级的 Go 数据验证库，支持结构体标签验证和丰富的验证规则。

## 安装

```go
import "xiaoshiai.cn/common/validation"
```

## 快速开始

### 基本使用

```go
package main

import (
    "context"
    "fmt"
    "xiaoshiai.cn/common/validation"
)

type User struct {
    Name  string `json:"name" validation:"required,len 1 50"`
    Email string `json:"email" validation:"required"`
    Age   int    `json:"age" validation:"range 0 150"`
    Role  string `json:"role" validation:"in admin user guest"`
    Port  int    `json:"port" validation:"port"`
}

func main() {
    v := validation.NewValidator()

    user := &User{
        Name: "",
        Age:  200,
        Role: "superadmin",
    }

    errs := v.Validate(context.Background(), user)
    if errs != nil {
        for _, err := range errs.Errors {
            fmt.Printf("Path: %s, Rule: %s, Params: %v\n", err.Path, err.Rule, err.Params)
        }
    }
}
```

### JSON 解析并验证

```go
jsonData := []byte(`{"name": "", "age": 200}`)
user := &User{}

errs := v.DecodeAndValidate(context.Background(), jsonData, user)
if errs != nil {
    // 处理验证错误
}
```

## 内置验证规则

| 规则       | 说明                   | 示例                                             |
| ---------- | ---------------------- | ------------------------------------------------ |
| `required` | 必填字段               | `validation:"required"`                          |
| `in`       | 枚举值验证             | `validation:"in a b c"`                          |
| `len`      | 长度验证（固定或范围） | `validation:"len 10"` 或 `validation:"len 1 50"` |
| `min`      | 最小值/最小长度        | `validation:"min 1"`                             |
| `max`      | 最大值/最大长度        | `validation:"max 100"`                           |
| `range`    | 数值范围               | `validation:"range 0 100"`                       |
| `regexp`   | 正则表达式             | `validation:"regexp ^[a-z]+$"`                   |
| `port`     | 端口验证 (1-65535)     | `validation:"port"`                              |

### 组合多个规则

使用逗号分隔多个规则：

```go
type Config struct {
    Name string `json:"name" validation:"required,len 1 63,regexp ^[a-z][a-z0-9-]*$"`
    Port int    `json:"port" validation:"required,port"`
}
```

## 自定义验证规则

```go
v := validation.NewValidator()

// 注册自定义规则
v.RegisterRule("even", func(ctx context.Context, value any, params ...string) *validation.ValidationError {
    if num, ok := value.(int); ok && num%2 != 0 {
        return validation.NewValidationError("even", nil, num)
    }
    return nil
})
```

## 验证错误结构

```go
type ValidationError struct {
    Path   string   `json:"path"`   // JSON Pointer 路径，如 "/spec/containers/0/name"
    Rule   string   `json:"rule"`   // 规则名称，如 "required"
    Params []string `json:"params"` // 规则参数
    Actual any      `json:"actual"` // 实际值
}
```

## 字符串验证工具函数

提供常用的字符串验证函数：

```go
validation.IsEmail("test@example.com")     // Email 验证
validation.IsURL("https://example.com")    // URL 验证
validation.IsIP("192.168.1.1")             // IP 地址验证
validation.IsIPv4("192.168.1.1")           // IPv4 验证
validation.IsIPv6("::1")                   // IPv6 验证
validation.IsCIDR("192.168.1.0/24")        // CIDR 验证
validation.IsDNSName("example.com")        // DNS 名称验证
validation.IsHost("example.com")           // 主机名验证 (IP 或 DNS)
validation.IsPort("8080")                  // 端口验证
validation.IsSemver("v1.2.3")              // 语义化版本验证
validation.IsBase64("aGVsbG8=")            // Base64 验证
validation.IsAlpha("abc")                  // 纯字母验证
validation.IsNumeric("123")                // 纯数字验证
validation.IsAlphanumeric("abc123")        // 字母数字验证
validation.IsRFC3339("2024-01-01T00:00:00Z") // RFC3339 时间验证
validation.IsValidEnvName("MY_VAR")        // 环境变量名验证
```

## 字段路径工具

支持点分隔格式和 JSON Pointer (RFC 6901) 格式的路径转换：

```go
// 创建字段路径
path := validation.NewFieldPath().
    AppendField("spec").
    AppendField("containers").
    AppendIndex(0).
    AppendField("name")

path.DotNotation()  // "spec.containers[0].name"
path.JSONPointer()  // "/spec/containers/0/name"

// 解析路径
validation.ParseDotNotation("spec.containers[0].name")
validation.ParseJSONPointer("/spec/containers/0/name")

// 格式转换
validation.DotToJSONPointer("spec.containers[0].name")  // "/spec/containers/0/name"
validation.JSONPointerToDot("/spec/containers/0/name")  // "spec.containers[0].name"
```
