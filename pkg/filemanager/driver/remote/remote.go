package remote

import (
	"context"
	"errors"
	"fmt"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/auth"
	"github.com/cloudreve/Cloudreve/v4/pkg/boolset"
	"github.com/cloudreve/Cloudreve/v4/pkg/cluster/routes"
	"github.com/cloudreve/Cloudreve/v4/pkg/conf"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/driver"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
	"github.com/cloudreve/Cloudreve/v4/pkg/request"
	"github.com/cloudreve/Cloudreve/v4/pkg/setting"
	"net/url"
	"os"
	"path"
	"time"
)

var (
	features = &boolset.BooleanSet{}
)

// Driver 远程存储策略适配器
type Driver struct {
	Client       request.Client
	Policy       *ent.StoragePolicy
	AuthInstance auth.Auth

	uploadClient Client
	config       conf.ConfigProvider
	settings     setting.Provider
}

// New initializes a new Driver from policy
func New(ctx context.Context, policy *ent.StoragePolicy, settings setting.Provider,
	config conf.ConfigProvider, l logging.Logger) (*Driver, error) {
	client, err := NewClient(ctx, policy, settings, config, l)
	if err != nil {
		return nil, err
	}

	return &Driver{
		Policy:       policy,
		Client:       request.NewClient(config),
		AuthInstance: auth.HMACAuth{[]byte(policy.Edges.Node.SlaveKey)},
		uploadClient: client,
		settings:     settings,
		config:       config,
	}, nil
}

//// List 列取文件
//func (handler *Driver) List(ctx context.Context, path string, recursive bool) ([]response.Object, error) {
//	var res []response.Object
//
//	reqBody := serializer.ListRequest{
//		Path:      path,
//		Recursive: recursive,
//	}
//	reqBodyEncoded, err := json.Marshal(reqBody)
//	if err != nil {
//		return res, err
//	}
//
//	// 发送列表请求
//	bodyReader := strings.NewReader(string(reqBodyEncoded))
//	signTTL := model.GetIntSetting("slave_api_timeout", 60)
//	resp, err := handler.Client.Request(
//		"POST",
//		handler.getAPIUrl("list"),
//		bodyReader,
//		request.WithCredential(handler.AuthInstance, int64(signTTL)),
//		request.WithMasterMeta(handler.settings.SiteBasic(ctx).ID, handler.settings.SiteURL(setting.UseFirstSiteUrl(ctx)).String()),
//	).CheckHTTPResponse(200).DecodeResponse()
//	if err != nil {
//		return res, err
//	}
//
//	// 处理列取结果
//	if resp.Code != 0 {
//		return res, errors.New(resp.Error)
//	}
//
//	if resStr, ok := resp.Data.(string); ok {
//		err = json.Unmarshal([]byte(resStr), &res)
//		if err != nil {
//			return res, err
//		}
//	}
//
//	return res, nil
//}

// getAPIUrl 获取接口请求地址
func (handler *Driver) getAPIUrl(scope string, routes ...string) string {
	serverURL, err := url.Parse(handler.Policy.Edges.Node.Server)
	if err != nil {
		return ""
	}
	var controller *url.URL

	switch scope {
	case "delete":
		controller, _ = url.Parse("/api/v3/slave/delete")
	case "thumb":
		controller, _ = url.Parse("/api/v3/slave/thumb")
	case "list":
		controller, _ = url.Parse("/api/v3/slave/list")
	default:
		controller = serverURL
	}

	for _, r := range routes {
		controller.Path = path.Join(controller.Path, r)
	}

	return serverURL.ResolveReference(controller).String()
}

// Open 获取文件内容
func (handler *Driver) Open(ctx context.Context, path string) (*os.File, error) {
	//// 尝试获取速度限制
	//speedLimit := 0
	//if user, ok := ctx.Value(fsctx.UserCtx).(model.User); ok {
	//	speedLimit = user.Group.SpeedLimit
	//}
	//
	//// 获取文件源地址
	//downloadURL, err := handler.Source(ctx, path, nil, true, int64(speedLimit))
	//if err != nil {
	//	return nil, err
	//}
	//
	//// 获取文件数据流
	//resp, err := handler.Client.Request(
	//	"GET",
	//	downloadURL,
	//	nil,
	//	request.WithContext(ctx),
	//	request.WithTimeout(time.Duration(0)),
	//	request.WithMasterMeta(handler.settings.SiteBasic(ctx).ID, handler.settings.SiteURL(ctx).String()),
	//).CheckHTTPResponse(200).GetRSCloser()
	//if err != nil {
	//	return nil, err
	//}
	//
	//resp.SetFirstFakeChunk()
	//
	//// 尝试获取文件大小
	//if file, ok := ctx.Value(fsctx.FileModelCtx).(model.File); ok {
	//	resp.SetContentLength(int64(file.Size))
	//}

	return nil, errors.New("not implemented")
}

func (handler *Driver) LocalPath(ctx context.Context, path string) string {
	return ""
}

// Put 将文件流保存到指定目录
func (handler *Driver) Put(ctx context.Context, file *fs.UploadRequest) error {
	defer file.Close()

	return handler.uploadClient.Upload(ctx, file)
}

// Delete 删除一个或多个文件，
// 返回未删除的文件，及遇到的最后一个错误
func (handler *Driver) Delete(ctx context.Context, files ...string) ([]string, error) {
	failed, err := handler.uploadClient.DeleteFiles(ctx, files...)
	if err != nil {
		return failed, err
	}
	return []string{}, nil
}

// Thumb 获取文件缩略图
func (handler *Driver) Thumb(ctx context.Context, expire *time.Time, ext string, e fs.Entity) (string, error) {
	serverURL, err := url.Parse(handler.Policy.Edges.Node.Server)
	if err != nil {
		return "", fmt.Errorf("parse server url failed: %w", err)
	}

	thumbURL := routes.SlaveThumbUrl(serverURL, e.Source(), ext)
	signedThumbURL, err := auth.SignURI(ctx, handler.AuthInstance, thumbURL.String(), expire)
	if err != nil {
		return "", err
	}

	return signedThumbURL.String(), nil
}

// Source 获取外链URL
func (handler *Driver) Source(ctx context.Context, e fs.Entity, args *driver.GetSourceArgs) (string, error) {
	server, err := url.Parse(handler.Policy.Edges.Node.Server)
	if err != nil {
		return "", err
	}

	nodeId := 0
	if handler.config.System().Mode == conf.SlaveMode {
		nodeId = handler.Policy.NodeID
	}

	base := routes.SlaveFileContentUrl(
		server,
		e.Source(),
		args.DisplayName,
		args.IsDownload,
		args.Speed,
		nodeId,
	)
	internalProxyed, err := auth.SignURI(ctx, handler.AuthInstance, base.String(), args.Expire)
	if err != nil {
		return "", fmt.Errorf("failed to sign internal slave content URL: %w", err)
	}

	return internalProxyed.String(), nil
}

// Token 获取上传策略和认证Token
func (handler *Driver) Token(ctx context.Context, uploadSession *fs.UploadSession, file *fs.UploadRequest) (*fs.UploadCredential, error) {
	siteURL := handler.settings.SiteURL(setting.UseFirstSiteUrl(ctx))
	// 在从机端创建上传会话
	uploadSession.Callback = routes.MasterSlaveCallbackUrl(siteURL, types.PolicyTypeRemote, uploadSession.Props.UploadSessionID, uploadSession.CallbackSecret).String()
	if err := handler.uploadClient.CreateUploadSession(ctx, uploadSession, false); err != nil {
		return nil, err
	}

	// 获取上传地址
	uploadURL, sign, err := handler.uploadClient.GetUploadURL(ctx, uploadSession.Props.ExpireAt, uploadSession.Props.UploadSessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to sign upload url: %w", err)
	}

	return &fs.UploadCredential{
		SessionID:  uploadSession.Props.UploadSessionID,
		ChunkSize:  handler.Policy.Settings.ChunkSize,
		UploadURLs: []string{uploadURL},
		Credential: sign,
	}, nil
}

// 取消上传凭证
func (handler *Driver) CancelToken(ctx context.Context, uploadSession *fs.UploadSession) error {
	return handler.uploadClient.DeleteUploadSession(ctx, uploadSession.Props.UploadSessionID)
}

func (handler *Driver) CompleteUpload(ctx context.Context, session *fs.UploadSession) error {
	return nil
}

func (handler *Driver) Capabilities() *driver.Capabilities {
	return &driver.Capabilities{
		StaticFeatures:         features,
		MediaMetaSupportedExts: handler.Policy.Settings.MediaMetaExts,
		MediaMetaProxy:         handler.Policy.Settings.MediaMetaGeneratorProxy,
		ThumbSupportedExts:     handler.Policy.Settings.ThumbExts,
		ThumbProxy:             handler.Policy.Settings.ThumbGeneratorProxy,
		ThumbMaxSize:           handler.Policy.Settings.ThumbMaxSize,
		ThumbSupportAllExts:    handler.Policy.Settings.ThumbSupportAllExts,
	}
}

func (handler *Driver) MediaMeta(ctx context.Context, path, ext string) ([]driver.MediaMeta, error) {
	return handler.uploadClient.MediaMeta(ctx, path, ext)
}
