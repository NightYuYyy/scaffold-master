package framework

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

// Principal 对应 Spring Security 中的 Authentication(principal=username, credentials=token)。
type Principal struct {
	Username string
	Token    string
}

// CurrentPrincipal 取当前登录主体；未登录返回 nil。
func CurrentPrincipal(ctx context.Context) *Principal {
	p, _ := ctx.Value(ctxPrincipal).(*Principal)
	return p
}

// CurrentUsername 取当前登录账号；未登录返回空串（Java 中为 null）。
func CurrentUsername(ctx context.Context) string {
	if p := CurrentPrincipal(ctx); p != nil {
		return p.Username
	}
	return ""
}

// ---------- 密码 ----------

// HashPassword 对应 BCryptPasswordEncoder.encode（强度 10，默认）；
// 超过 72 字节时 Java 抛 IllegalArgumentException → GlobalExceptionHandler 返回 400 + 原文。
func HashPassword(raw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(raw), bcrypt.DefaultCost)
	if errors.Is(err, bcrypt.ErrPasswordTooLong) {
		return "", Biz("password cannot be more than 72 bytes")
	}
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// PasswordMatches 对应 PasswordEncoder.matches；库中 $2a/$2b 均可校验。
func PasswordMatches(raw, hashed string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(raw)) == nil
}

// ---------- JWT ----------

// JWT 对应 JwtService：claims 为 jti/sub/userId/iat/exp。
// 签名算法与 jjwt Keys.hmacShaKeyFor 一致：按密钥长度选 HS256（<48 字节）/ HS384（<64）/ HS512（≥64），
// 这样 Java 与 Go 连同一个 Redis 时彼此签发的 token 可以互认。
type JWT struct {
	secret  []byte
	method  *jwt.SigningMethodHMAC
	Expires time.Duration
}

// NewJWT 校验密钥：UTF-8 至少 32 字节且不能全为空白。
func NewJWT(secret string, expiresSeconds int64) (*JWT, error) {
	if strings.TrimSpace(secret) == "" || len([]byte(secret)) < 32 {
		return nil, errors.New("JWT_SECRET 必须配置为至少 32 字节的非空白密钥")
	}
	method := jwt.SigningMethodHS256
	if len(secret) >= 64 {
		method = jwt.SigningMethodHS512
	} else if len(secret) >= 48 {
		method = jwt.SigningMethodHS384
	}
	return &JWT{secret: []byte(secret), method: method, Expires: time.Duration(expiresSeconds) * time.Second}, nil
}

func (j *JWT) ExpiresSeconds() int64 { return int64(j.Expires / time.Second) }

func (j *JWT) Generate(userID int64, username string) string {
	now := time.Now()
	t := jwt.NewWithClaims(j.method, jwt.MapClaims{
		"jti":    UUIDHex(),
		"sub":    username,
		"userId": userID,
		"iat":    now.Unix(),
		"exp":    now.Add(j.Expires).Unix(),
	})
	s, err := t.SignedString(j.secret)
	if err != nil {
		panic(err)
	}
	return s
}

// ParseUsername 校验签名与过期时间，失败返回空串（Java 返回 null）。
func (j *JWT) ParseUsername(token string) string {
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return j.secret, nil
	}, jwt.WithValidMethods([]string{"HS256", "HS384", "HS512"}))
	if err != nil {
		return ""
	}
	sub, _ := claims["sub"].(string)
	return sub
}

// ---------- Redis：Token 与验证码 ----------

// TokenStore 对应 Java TokenStore，key = auth:token:<token>。
type TokenStore struct{ rdb *redis.Client }

func NewTokenStore(rdb *redis.Client) *TokenStore { return &TokenStore{rdb: rdb} }

func (s *TokenStore) Store(ctx context.Context, token, username string, expires time.Duration) error {
	return s.rdb.Set(ctx, "auth:token:"+token, username, expires).Err()
}

// UsernameOf 返回 token 对应账号；不存在返回空串。
func (s *TokenStore) UsernameOf(ctx context.Context, token string) string {
	v, err := s.rdb.Get(ctx, "auth:token:"+token).Result()
	if err != nil {
		return ""
	}
	return v
}

func (s *TokenStore) Revoke(ctx context.Context, token string) {
	_ = s.rdb.Del(ctx, "auth:token:"+token).Err()
}

// Captcha 对应 CaptchaService：4 位字符，Redis 保存 300 秒。
type Captcha struct{ rdb *redis.Client }

func NewCaptcha(rdb *redis.Client) *Captcha { return &Captcha{rdb: rdb} }

const captchaChars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func (c *Captcha) Generate(ctx context.Context) (map[string]string, error) {
	uuid := UUIDHex()
	code := make([]byte, 4)
	for i := range code {
		code[i] = captchaChars[rand.IntN(len(captchaChars))]
	}
	if err := c.rdb.Set(ctx, "captcha:"+uuid, string(code), 300*time.Second).Err(); err != nil {
		return nil, err
	}
	return map[string]string{"uuid": uuid, "code": string(code)}, nil
}

// Verify 取出并删除验证码，忽略大小写比较。
func (c *Captcha) Verify(ctx context.Context, uuid, code *string) bool {
	if uuid == nil || code == nil {
		return false
	}
	key := "captcha:" + *uuid
	stored, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		return false
	}
	_ = c.rdb.Del(ctx, key).Err()
	return strings.EqualFold(stored, *code)
}

// ---------- 认证与权限 ----------

// PermissionResolver 对应 Java PermissionResolver，由 system 包实现。
type PermissionResolver interface {
	HasPermission(ctx context.Context, username, permission string) bool
}

// Security 聚合认证所需组件。
type Security struct {
	JWT        *JWT
	Tokens     *TokenStore
	Permission PermissionResolver
	// PermitAll 为无需登录的精确路径（对应 SecurityConfig.permitAll）。
	PermitAll map[string]bool
}

// Authenticate 对应 JwtAuthenticationFilter + SecurityFilterChain：
// 解析 Bearer token 并校验 Redis 中仍有效，写入 Principal；非白名单且未认证 → 401。
func (s *Security) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			token := auth[len("Bearer "):]
			if username := s.JWT.ParseUsername(token); username != "" && s.Tokens.UsernameOf(r.Context(), token) == username {
				r = r.WithContext(context.WithValue(r.Context(), ctxPrincipal, &Principal{Username: username, Token: token}))
			}
		}
		// Spring Security 默认响应头
		h := w.Header()
		h.Set("Cache-Control", "no-cache, no-store, max-age=0, must-revalidate")
		h.Set("Pragma", "no-cache")
		h.Set("Expires", "0")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-XSS-Protection", "0")
		if CurrentPrincipal(r.Context()) == nil && !s.PermitAll[r.URL.Path] {
			// RestAuthenticationEntryPoint 显式设置了 charset；其余响应由 Spring MVC 写出，不带 charset
			w.Header().Set("Content-Type", "application/json;charset=UTF-8")
			_ = WriteJSON(w, 401, R{Code: 401, Message: ErrUnauthorized.Message, TraceID: TraceID(r.Context())})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Perm 对应 @RequiresPermission：无权限 → 403。
func (s *Security) Perm(permission string) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			username := CurrentUsername(r.Context())
			if username == "" || !s.Permission.HasPermission(r.Context(), username, permission) {
				return ErrForbidden
			}
			return next(w, r)
		}
	}
}

// ClientIP 对应 OperationLogAspect.clientIp。
func ClientIP(r *http.Request) string {
	if f := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); f != "" {
		return strings.TrimSpace(strings.Split(f, ",")[0])
	}
	if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" {
		return real
	}
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return javaHostAddress(host)
}

// javaHostAddress 对应 InetAddress.getHostAddress()：IPv6 展开为 8 组无前导零的十六进制（如 0:0:0:0:0:0:0:1），
// IPv4 映射地址还原为点分十进制。
func javaHostAddress(host string) string {
	ip := net.ParseIP(host)
	if ip == nil {
		return host
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	ip = ip.To16()
	parts := make([]string, 8)
	for i := range parts {
		parts[i] = strconv.FormatUint(uint64(ip[2*i])<<8|uint64(ip[2*i+1]), 16)
	}
	return strings.Join(parts, ":")
}

// String 便于日志输出。
func (p *Principal) String() string { return fmt.Sprintf("%s", p.Username) }
