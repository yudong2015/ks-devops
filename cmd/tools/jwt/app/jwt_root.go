/*
Copyright 2022 The KubeSphere Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package app

import (
	"context"
	"fmt"
	v1 "k8s.io/api/core/v1"
	"kubesphere.io/devops/pkg/client/k8s"
	"math/rand"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/kubernetes"
	"kubesphere.io/devops/pkg/config"
	"kubesphere.io/devops/pkg/jwt/token"
)

// NewCmd creates a root command for jwt
func NewCmd() (cmd *cobra.Command) {
	opt := &jwtOption{}

	cmd = &cobra.Command{
		Use:     "jwt",
		Short:   "Output the JWT",
		PreRunE: opt.preRunE,
		RunE:    opt.runE,
	}

	flags := cmd.Flags()
	flags.StringVarP(&opt.secret, "secret", "s", "",
		"The secret for generating jwt")
	flags.StringVarP(&opt.namespace, "namespace", "", "kubesphere-devops-system",
		"The namespace of target ConfigMap")
	flags.StringVarP(&opt.name, "name", "", "devops-config",
		"The name of target ConfigMap")
	flags.StringVarP(&opt.output, "output", "o", "",
		"The destination of the JWT output. Print to the stdout if it's empty.")
	flags.BoolVarP(&opt.overrideJenkinsToken, "override-jenkins-token", "", true,
		"If you want to override the Jenkins token.")
	return
}

type jwtOption struct {
	secret               string
	output               string
	overrideJenkinsToken bool

	namespace string
	name      string

	client           kubernetes.Interface
	configMapUpdater configMapUpdater
}

func (o *jwtOption) preRunE(cmd *cobra.Command, args []string) (err error) {
	if o.output == "configmap" || o.secret == "" {
		var client k8s.Client
		if client, err = k8s.NewKubernetesClient(k8s.NewKubernetesOptions()); err != nil {
			err = fmt.Errorf("cannot create Kubernetes client, error: %v", err)
			return
		}
		o.client = client.Kubernetes()
		o.configMapUpdater = o
	}

	// get secret from ConfigMap if it's empty
	if o.secret == "" {
		if o.secret, err = o.getSecret(); o.secret == "" {
			// generate a new secret if the ConfigMap does not contain it, then update it into ConfigMap
			o.updateSecret(o.generateSecret())
		}
	}
	return
}

func (o *jwtOption) getSecret() (secret string, err error) {
	var cm *v1.ConfigMap
	if cm, err = o.configMapUpdater.GetConfigMap(context.TODO(), o.namespace, o.name); err == nil {
		if data, ok := cm.Data[config.DefaultConfigurationFileName]; ok {
			dataMap := make(map[string]map[string]string, 0)
			if err = yaml.Unmarshal([]byte(data), dataMap); err == nil {
				if _, ok := dataMap["authentication"]; ok {
					secret = dataMap["authentication"]["jwtSecret"]
				}
			}
		}
	}
	return
}

func (o *jwtOption) generateSecret() string {
	rand.Seed(time.Now().UnixNano())
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, 32)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func (o *jwtOption) updateSecret(secret string) {
	ctx := context.TODO()
	if cm, err := o.configMapUpdater.GetConfigMap(ctx, o.namespace, o.name); err == nil {
		if data, ok := cm.Data[config.DefaultConfigurationFileName]; ok {
			dataMap := make(map[string]map[string]string, 0)
			if err := yaml.Unmarshal([]byte(data), dataMap); err == nil {
				if _, ok := dataMap["authentication"]; ok {
					dataMap["authentication"]["jwtSecret"] = secret
				} else {
					dataMap["authentication"] = map[string]string{
						"jwtSecret": secret,
					}
				}

				cfg, _ := yaml.Marshal(dataMap)
				cm.Data[config.DefaultConfigurationFileName] = string(cfg)
				_, _ = o.configMapUpdater.UpdateConfigMap(ctx, cm)
			}
		}
	}
}

func (o *jwtOption) runE(cmd *cobra.Command, args []string) (err error) {
	jwt := generateJWT(o.secret)

	switch o.output {
	case "configmap":
		err = o.updateJenkinsToken(jwt, o.namespace, o.name)
	default:
		cmd.Print(jwt)
	}
	return
}

func updateToken(content, token string, override bool) string {
	dataMap := make(map[string]map[string]string, 0)
	if err := yaml.Unmarshal([]byte(content), dataMap); err == nil {
		if _, ok := dataMap["devops"]; ok {
			if dataMap["devops"]["password"] != "" && override {
				dataMap["devops"]["password"] = token
			}

			if result, err := yaml.Marshal(dataMap); err == nil {
				return strings.TrimSpace(string(result))
			}
		}
	}
	return content
}

func generateJWT(secret string) (jwt string) {
	issuer := token.NewTokenIssuer(secret, 0)
	admin := &user.DefaultInfo{
		Name: "admin",
	}

	jwt, _ = issuer.IssueTo(admin, token.AccessToken, 0)
	return
}
