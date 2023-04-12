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
	"bytes"
	"context"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"kubesphere.io/devops/pkg/config"
	"testing"
)

func Test_generateJWT(t *testing.T) {
	type args struct {
		secret string
	}
	tests := []struct {
		name    string
		args    args
		wantJwt string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotJwt := generateJWT(tt.args.secret); gotJwt != tt.wantJwt {
				t.Errorf("generateJWT() = %v, want %v", gotJwt, tt.wantJwt)
			}
		})
	}
}

func Test_updateToken(t *testing.T) {
	type args struct {
		content  string
		token    string
		override bool
	}
	tests := []struct {
		name string
		args args
		want string
	}{{
		name: "invalid yaml content",
		args: args{
			content:  "fake",
			override: true,
		},
		want: "fake",
	}, {
		name: "valid yaml without devops.token",
		args: args{
			content:  "name: rick",
			override: true,
		},
		want: "name: rick",
	}, {
		name: "valid yaml with devops.token",
		args: args{
			content: `devops:
  password: fake`,
			token:    "token",
			override: true,
		},
		want: `devops:
  password: token`,
	}, {
		name: "do not override",
		args: args{
			content: `devops:
  password: fake`,
			token:    "token",
			override: false,
		},
		want: `devops:
  password: fake`,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := updateToken(tt.args.content, tt.args.token, tt.args.override); got != tt.want {
				t.Errorf("updateToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

var _ = Describe("", func() {
	var (
		ctrl *gomock.Controller
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
	})

	AfterEach(func() {
		defer ctrl.Finish()
	})

	Context("stdout case", func() {
		It("should success", func() {
			cmd := NewCmd()
			cmd.SetOut(bytes.NewBuffer([]byte{}))
			_, err := cmd.ExecuteC()
			Expect(err).NotTo(BeNil())
		})
	})

	Context("update ConfigMap case", func() {
		It("cannot get k8s client", func() {
			cmd := NewCmd()
			cmd.SetOut(bytes.NewBuffer([]byte{}))
			cmd.SetErr(bytes.NewBuffer([]byte{}))
			cmd.SetArgs([]string{"jwt", "--output", "configmap"})
			_, err := cmd.ExecuteC()

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot create Kubernetes client"))
		})

		It("cannot find configmap", func() {
			cmd := NewCmd()
			cmd.SetOut(bytes.NewBuffer([]byte{}))
			cmd.SetErr(bytes.NewBuffer([]byte{}))
			cmd.SetArgs([]string{"jwt", "--output", "configmap"})
			_, err := cmd.ExecuteC()

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("no kubesphere.yaml found", func() {
			cmd := NewCmd()
			cmd.SetOut(bytes.NewBuffer([]byte{}))
			cmd.SetErr(bytes.NewBuffer([]byte{}))
			cmd.SetArgs([]string{"jwt", "--output", "configmap"})
			_, err := cmd.ExecuteC()

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no kubesphere.yaml found"))
		})

		It("has invalid kubesphere.yaml", func() {
			data := map[string]string{
				config.DefaultConfigurationFileName: `name: rick`,
			}
			client := fake.NewSimpleClientset(&v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "kubesphere-devops-system",
					Name:      "devops-config",
				},
				Data: data,
			})

			cmd := NewCmd()
			cmd.SetOut(bytes.NewBuffer([]byte{}))
			cmd.SetErr(bytes.NewBuffer([]byte{}))
			cmd.SetArgs([]string{"jwt", "--output", "configmap"})
			_, err := cmd.ExecuteC()

			Expect(err).To(HaveOccurred())

			// make sure the original ConfigMap data has not changed
			opt := jwtOption{
				client: client,
			}
			ctx := context.TODO()
			cm, err := opt.GetConfigMap(ctx, "kubesphere-devops-system", "devops-config")
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Data).To(Equal(data))
		})

		It("has valid kubesphere.yaml without jwtSecret", func() {
			data := map[string]string{
				config.DefaultConfigurationFileName: `devops:
  password: xxx`,
			}
			client := fake.NewSimpleClientset(&v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "kubesphere-devops-system",
					Name:      "devops-config",
				},
				Data: data,
			})

			cmd := NewCmd()
			cmd.SetOut(bytes.NewBuffer([]byte{}))
			cmd.SetErr(bytes.NewBuffer([]byte{}))
			cmd.SetArgs([]string{"jwt", "--output", "configmap"})
			_, err := cmd.ExecuteC()

			Expect(err).NotTo(HaveOccurred())

			// make sure the original ConfigMap data has changed
			opt := jwtOption{
				client: client,
			}
			ctx := context.TODO()
			cm, err := opt.GetConfigMap(ctx, "kubesphere-devops-system", "devops-config")
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Data).NotTo(Equal(data))
			Expect(cm.Data[config.DefaultConfigurationFileName]).To(ContainSubstring("jwtSecret"))
		})

		It("has valid kubesphere.yaml with jwtSecret", func() {
			data := map[string]string{
				config.DefaultConfigurationFileName: `authentication:
  jwtSecret:
devops:
  password: xxx`,
			}
			client := fake.NewSimpleClientset(&v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "kubesphere-devops-system",
					Name:      "devops-config",
				},
				Data: data,
			})

			cmd := NewCmd()
			cmd.SetOut(bytes.NewBuffer([]byte{}))
			cmd.SetErr(bytes.NewBuffer([]byte{}))
			cmd.SetArgs([]string{"jwt", "--output", "configmap"})
			_, err := cmd.ExecuteC()

			Expect(err).NotTo(HaveOccurred())

			// make sure the original ConfigMap data has changed
			opt := jwtOption{
				client: client,
			}
			ctx := context.TODO()
			cm, err := opt.GetConfigMap(ctx, "kubesphere-devops-system", "devops-config")
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Data).NotTo(Equal(data))
			Expect(cm.Data[config.DefaultConfigurationFileName]).To(ContainSubstring("jwtSecret:"))
		})
	})
})

func Test_jwtOption_generateSecret(t *testing.T) {
	o := &jwtOption{}

	if got := o.generateSecret(); got == "" {
		t.Fatalf("generateSecret() should not return an empty string")
	} else if len(got) != 32 {
		t.Fatalf("generateSecret() should return an string with 32 letters")
	}

	// the secret should be a dynamic value
	secretMap := make(map[string]string)
	for i := 0; i < 30; i++ {
		secret := o.generateSecret()
		if _, ok := secretMap[secret]; ok {
			t.Fatalf("found duplicated secret: %s", secret)
		} else {
			secretMap[secret] = ""
		}
	}
}
