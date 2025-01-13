package provisioners

import "sync"

var secretAccessedMutex sync.Mutex
var secretAccessed bool

func BuildSubstitutionFuncWithSecretWatch(inner func(string) (string, error)) (func(string) (string, error), *bool) {
	var localSecretAccessed bool
	return func(s string) (string, error) {
		secretAccessedMutex.Lock()
		defer secretAccessedMutex.Unlock()
		v, err := inner(s)
		if secretAccessed {
			localSecretAccessed = true
			secretAccessed = false
		}
		return v, err
	}, &localSecretAccessed
}

func MarkSecretAccessed() {
	secretAccessed = true
}
