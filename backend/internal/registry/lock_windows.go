//go:build windows

package registry

import "errors"

func Lock(name string) (func(), error) {
	return nil, errors.New("运维锁仅支持 Linux，请在 Linux 容器或服务器执行")
}
