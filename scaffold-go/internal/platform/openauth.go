package platform

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"

	"scaffold/internal/framework"
)

const tokenIssuer = "scaffold-platform"

var (
	errAppInvalid   = &framework.Error{Status: 401, Code: 401, Message: "应用或密钥无效"}
	errAppSecret    = &framework.Error{Status: 401, Code: 401, Message: "应用密钥错误"}
	errUserDisabled = &framework.Error{Status: 401, Code: 401, Message: "账号已停用"}
)

type ctxKey int

const ctxOpen ctxKey = iota

// openCtx 开放接口的认证上下文：应用 + 密钥（+ 用户令牌对应的用户与 jti）。
type openCtx struct {
	app  *openApp
	key  *openKey
	user *appUser
	jti  string
}

func openOf(ctx context.Context) *openCtx {
	oc, _ := ctx.Value(ctxOpen).(*openCtx)
	return oc
}

func tokenKey(jti string) string { return "open:token:" + jti }

func userTokensKey(appID, userID int64) string {
	return "open:user:" + strconv.FormatInt(appID, 10) + ":" + strconv.FormatInt(userID, 10)
}

// resolveKey X-App-Key → 应用与密钥（均须启用、应用未删除），并更新密钥最近使用时间。
func (s *svc) resolveKey(ctx context.Context, apiKey string) (*openApp, *openKey, error) {
	if apiKey == "" {
		return nil, nil, errAppInvalid
	}
	app, key, err := loadKey(ctx, s.db, apiKey)
	if err != nil {
		return nil, nil, err
	}
	if app == nil || app.Status != 1 || key.Status != 1 {
		return nil, nil, errAppInvalid
	}
	_, _ = framework.Exec(ctx, s.db, `UPDATE app_api_key SET last_used_time = NOW() WHERE id = ?`, key.ID)
	return app, key, nil
}

// appKey 中间件：X-App-Key。
func (s *svc) appKey(next framework.HandlerFunc) framework.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		app, key, err := s.resolveKey(r.Context(), r.Header.Get("X-App-Key"))
		if err != nil {
			return err
		}
		return next(w, r.WithContext(context.WithValue(r.Context(), ctxOpen, &openCtx{app: app, key: key})))
	}
}

// appSecret 中间件：X-App-Key + X-App-Secret（常量时间比较）。
func (s *svc) appSecret(next framework.HandlerFunc) framework.HandlerFunc {
	return s.appKey(func(w http.ResponseWriter, r *http.Request) error {
		oc := openOf(r.Context())
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-App-Secret")), []byte(oc.key.Secret)) != 1 {
			return errAppSecret
		}
		return next(w, r)
	})
}

// userToken 中间件：Authorization: Bearer <用户 JWT>。
func (s *svc) userToken(next framework.HandlerFunc) framework.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			return framework.ErrUnauthorized
		}
		info, err := s.parseToken(r.Context(), strings.TrimSpace(auth[len("Bearer "):]))
		if err != nil {
			return err
		}
		u, err := loadAppUser(r.Context(), s.db, info.app.ID, info.userID)
		if err != nil {
			return err
		}
		if u == nil {
			return framework.ErrUnauthorized
		}
		if u.Status != 1 {
			return errUserDisabled
		}
		oc := &openCtx{app: info.app, key: info.key, user: u, jti: info.jti}
		return next(w, r.WithContext(context.WithValue(r.Context(), ctxOpen, oc)))
	}
}

// tokenInfo 验签通过的用户令牌。
type tokenInfo struct {
	app    *openApp
	key    *openKey
	userID int64
	jti    string
	exp    time.Time
}

// parseToken 按头部 kid 取密钥验签（HS256），校验 iss / aud / exp，并确认 Redis 中 jti 仍有效。
// 验签、声明或 Redis 校验失败均返回 ErrUnauthorized；数据库错误原样返回。
func (s *svc) parseToken(ctx context.Context, raw string) (*tokenInfo, error) {
	var app *openApp
	var key *openKey
	var dbErr error
	claims := &jwt.RegisteredClaims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("缺少 kid")
		}
		a, k, err := loadKey(ctx, s.db, kid)
		if err != nil {
			dbErr = err
			return nil, err
		}
		if a == nil || a.Status != 1 || k.Status != 1 {
			return nil, errors.New("密钥不可用")
		}
		app, key = a, k
		return []byte(k.Secret), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(tokenIssuer), jwt.WithExpirationRequired())
	if dbErr != nil {
		return nil, dbErr
	}
	if err != nil || app == nil || claims.ID == "" || !slices.Contains(claims.Audience, app.Code) {
		return nil, framework.ErrUnauthorized
	}
	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		return nil, framework.ErrUnauthorized
	}
	stored, err := s.rdb.Get(ctx, tokenKey(claims.ID)).Result()
	if errors.Is(err, redis.Nil) || (err == nil && stored != claims.Subject) {
		return nil, framework.ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	return &tokenInfo{app: app, key: key, userID: userID, jti: claims.ID, exp: claims.ExpiresAt.Time}, nil
}

// issueToken 签发用户令牌并登记到 Redis（open:token:<jti> 与用户 jti 集合）。
func (s *svc) issueToken(ctx context.Context, app *openApp, key *openKey, userID int64) (string, error) {
	now := time.Now()
	ttl := time.Duration(app.TokenExpires) * time.Second
	jti := framework.UUIDHex()
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": tokenIssuer,
		"aud": app.Code,
		"sub": strconv.FormatInt(userID, 10),
		"jti": jti,
		"iat": now.Unix(),
		"exp": now.Add(ttl).Unix(),
	})
	t.Header["kid"] = key.APIKey
	signed, err := t.SignedString([]byte(key.Secret))
	if err != nil {
		return "", err
	}
	setKey := userTokensKey(app.ID, userID)
	pipe := s.rdb.TxPipeline()
	pipe.Set(ctx, tokenKey(jti), userID, ttl)
	// 应用缩短有效期不能让撤销索引早于已签发的长效令牌过期。
	pipe.Eval(ctx, `
		redis.call('SADD', KEYS[1], ARGV[1])
		if redis.call('PTTL', KEYS[1]) < tonumber(ARGV[2]) then
			redis.call('PEXPIRE', KEYS[1], ARGV[2])
		end
		return 1`, []string{setKey}, jti, ttl.Milliseconds())
	_, err = pipe.Exec(ctx)
	return signed, err
}

// revokeToken 撤销单个令牌。
func (s *svc) revokeToken(ctx context.Context, appID, userID int64, jti string) error {
	pipe := s.rdb.TxPipeline()
	pipe.Del(ctx, tokenKey(jti))
	pipe.SRem(ctx, userTokensKey(appID, userID), jti)
	_, err := pipe.Exec(ctx)
	return err
}

// revokeUserTokens 撤销用户全部令牌。
func (s *svc) revokeUserTokens(ctx context.Context, appID, userID int64) error {
	setKey := userTokensKey(appID, userID)
	jtis, err := s.rdb.SMembers(ctx, setKey).Result()
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(jtis)+1)
	for _, jti := range jtis {
		keys = append(keys, tokenKey(jti))
	}
	return s.rdb.Del(ctx, append(keys, setKey)...).Err()
}
