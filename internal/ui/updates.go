package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"fyne.io/fyne/v2/dialog"
	"github.com/ealink1/navi-fyne/internal/infra/release"
	"github.com/ealink1/navi-fyne/internal/infra/update"
)

func (w *Window) checkUpdates() {
	w.status.SetText("正在检查 GitHub Release…")
	w.jobs.run(func(ctx context.Context) (any, error) {
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		return w.Releases.Check(ctx)
	}, func(value any, err error) {
		if errors.Is(err, release.ErrNoRelease) {
			w.status.SetText("目标仓库尚未发布 Release")
			return
		}
		if err != nil {
			w.showError(err)
			w.status.SetText("更新检查失败")
			return
		}
		manifest := value.(release.Manifest)
		if !release.Newer(manifest.Version, w.Version) {
			dialog.ShowInformation("检查更新", "当前已是最新稳定版本。", w.Window)
			return
		}
		artifact, err := manifest.Artifact("app", "navi-fyne", runtime.GOOS, runtime.GOARCH)
		if err != nil {
			w.showError(err)
			return
		}
		dialog.ShowConfirm("发现新版本", fmt.Sprintf("%s → %s\n签名清单验证通过。下载完整应用包（含配套驱动），大小 %.1f MiB？", w.Version, manifest.Version, float64(artifact.Size)/(1<<20)), func(ok bool) {
			if !ok {
				return
			}
			w.status.SetText("正在下载并校验完整应用包…")
			w.jobs.run(func(ctx context.Context) (any, error) {
				ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
				defer cancel()
				return w.Releases.Download(ctx, artifact, filepath.Join(w.Root, "updates", manifest.Version))
			}, func(value any, err error) {
				if err != nil {
					w.showError(err)
					return
				}
				w.installUpdate(value.(string), manifest.Version)
			})
		}, w.Window)
	})
}
func (w *Window) installUpdate(path, version string) {
	executable, err := os.Executable()
	if err != nil {
		w.showError(err)
		return
	}
	target, err := update.RunningRoot(executable)
	if err != nil {
		dialog.ShowInformation("更新包已校验", err.Error()+"\n下载位置：\n"+path, w.Window)
		return
	}
	dialog.ShowConfirm("安装并重启", "完整应用包已校验。保存草稿后退出并替换应用；旧版本会保留为备份。是否继续？", func(ok bool) {
		if !ok {
			return
		}
		w.status.SetText("正在准备应用替换…")
		w.jobs.run(func(ctx context.Context) (any, error) {
			return update.Prepare(ctx, path, filepath.Dir(path), target, version, w.Root)
		}, func(value any, err error) {
			if err != nil {
				w.showError(err)
				return
			}
			request := value.(update.Request)
			w.pendingUpdate = &request
			w.shutdown()
		})
	}, w.Window)
}
