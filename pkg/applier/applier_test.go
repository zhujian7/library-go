package applier

import (
	"context"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestApplierClient_CreateOrUpdateInPath(t *testing.T) {
	testscheme := scheme.Scheme

	testscheme.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.ClusterRole{})
	testscheme.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.ClusterRoleBinding{})
	testscheme.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.ServiceAccount{})

	tp, err := NewTemplateProcessor(NewTestReader(assets), nil)
	if err != nil {
		t.Errorf("Unable to create applier %s", err.Error())
	}

	client := fake.NewFakeClient([]runtime.Object{}...)

	a, err := NewApplier(tp, client, nil, nil, nil)
	if err != nil {
		t.Errorf("Unable to create applier %s", err.Error())
	}

	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      values.BootstrapServiceAccountName,
			Namespace: values.ManagedClusterNamespace,
		},
	}
	saSecrets := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      values.BootstrapServiceAccountName,
			Namespace: values.ManagedClusterNamespace,
		},
		Secrets: []corev1.ObjectReference{
			{Name: "objectname"},
		},
	}
	clientUpdate := fake.NewFakeClient(sa)

	aUpdate, err := NewApplier(tp, clientUpdate, nil, nil, DefaultKubernetesMerger)
	if err != nil {
		t.Errorf("Unable to create applier %s", err.Error())
	}

	clientUpdateNoMerger := fake.NewFakeClient(sa)

	aUpdateNoMerger, err := NewApplier(tp, clientUpdateNoMerger, nil, nil, nil)
	if err != nil {
		t.Errorf("Unable to create applier %s", err.Error())
	}

	clientUpdateMerged := fake.NewFakeClient(saSecrets)

	aUpdateMerged, err := NewApplier(tp, clientUpdateMerged, nil, nil, DefaultKubernetesMerger)
	if err != nil {
		t.Errorf("Unable to create applier %s", err.Error())
	}
	type args struct {
		path      string
		excluded  []string
		recursive bool
		values    interface{}
	}
	tests := []struct {
		name    string
		fields  Applier
		args    args
		wantErr bool
	}{
		{
			name:   "success",
			fields: *a,
			args: args{
				path:      "test",
				excluded:  nil,
				recursive: false,
				values:    values,
			},
			wantErr: false,
		},
		{
			name:   "success update",
			fields: *aUpdate,
			args: args{
				path:      "test",
				excluded:  nil,
				recursive: false,
				values:    values,
			},
			wantErr: false,
		},
		{
			name:   "success update no merger",
			fields: *aUpdateNoMerger,
			args: args{
				path:      "test",
				excluded:  nil,
				recursive: false,
				values:    values,
			},
			wantErr: true,
		},
		{
			name:   "success update merged",
			fields: *aUpdateMerged,
			args: args{
				path: "test",
				excluded: []string{
					"test/clusterrolebinding",
					"test/clusterrole",
				},
				recursive: false,
				values:    values,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fields.CreateOrUpdateInPath(tt.args.path, tt.args.excluded, tt.args.recursive, tt.args.values)
			if (err != nil) != tt.wantErr {
				t.Errorf("ApplierClient.CreateOrUpdateInPath() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil {
				sa := &corev1.ServiceAccount{}
				err := client.Get(context.TODO(), types.NamespacedName{
					Name:      values.BootstrapServiceAccountName,
					Namespace: values.ManagedClusterNamespace,
				}, sa)
				if err != nil {
					t.Error(err)
				}
				r := &rbacv1.ClusterRole{}
				err = client.Get(context.TODO(), types.NamespacedName{
					Name: values.ManagedClusterName,
				}, r)
				if err != nil {
					t.Error(err)
				}
				rb := &rbacv1.ClusterRoleBinding{}
				err = client.Get(context.TODO(), types.NamespacedName{
					Name: values.ManagedClusterName,
				}, rb)
				if err != nil {
					t.Error(err)
				}
				if rb.RoleRef.Name != values.ManagedClusterName {
					t.Errorf("Expecting %s got %s", values.ManagedClusterName, rb.RoleRef.Name)
				}
				switch tt.name {
				case "success update":
					if len(sa.Secrets) == 0 {
						t.Error("Not merged as no secrets found")
					}
				case "success update merged":
					if sa.Secrets[0].Name != "mysecret" {
						t.Errorf("Not merged secrets=%#v", sa.Secrets[0])
					}
				}
			}
		})
	}
}

func TestNewApplier(t *testing.T) {
	tp := &TemplateProcessor{}
	client := fake.NewFakeClient([]runtime.Object{}...)
	owner := &corev1.Secret{}
	scheme := &runtime.Scheme{}
	merger := func(current,
		new *unstructured.Unstructured,
	) (
		future *unstructured.Unstructured,
		update bool,
	) {
		return nil, true
	}
	type args struct {
		templateProcessor *TemplateProcessor
		client            crclient.Client
		owner             metav1.Object
		scheme            *runtime.Scheme
		merger            Merger
	}
	tests := []struct {
		name    string
		args    args
		want    *Applier
		wantErr bool
	}{
		{
			name: "Succeed",
			args: args{
				templateProcessor: tp,
				client:            client,
				owner:             owner,
				scheme:            scheme,
				merger:            merger,
			},
			want: &Applier{
				templateProcessor: tp,
				client:            client,
				owner:             owner,
				scheme:            scheme,
				merger:            merger,
			},
			wantErr: false,
		},
		{
			name: "Failed no templateProcessor",
			args: args{
				templateProcessor: nil,
				client:            client,
				owner:             owner,
				scheme:            scheme,
				merger:            merger,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Failed no client",
			args: args{
				templateProcessor: tp,
				client:            nil,
				owner:             owner,
				scheme:            scheme,
				merger:            merger,
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewApplier(tt.args.templateProcessor, tt.args.client, tt.args.owner, tt.args.scheme, tt.args.merger)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewApplier() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != nil {
				if !reflect.DeepEqual(got.templateProcessor, tt.want.templateProcessor) &&
					!reflect.DeepEqual(got.client, tt.want.client) &&
					!reflect.DeepEqual(got.owner, tt.want.owner) &&
					!reflect.DeepEqual(got.scheme, tt.want.scheme) &&
					!reflect.DeepEqual(got.merger, tt.want.merger) {
					t.Errorf("NewApplier() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestApplier_setControllerReference(t *testing.T) {
	testscheme := scheme.Scheme

	testscheme.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.ClusterRole{})
	testscheme.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.ClusterRoleBinding{})
	testscheme.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.ServiceAccount{})

	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "set-controller-reference",
			Namespace: values.ManagedClusterNamespace,
		},
	}

	type fields struct {
		templateProcessor *TemplateProcessor
		client            crclient.Client
		owner             metav1.Object
		scheme            *runtime.Scheme
		merger            Merger
	}
	type args struct {
		u *unstructured.Unstructured
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "success",
			fields: fields{
				client: nil,
				owner:  sa,
				scheme: testscheme,
				merger: nil,
			},
			args: args{
				u: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": corev1.SchemeGroupVersion.String(),
						"kind":       "ServiceAccount",
						"metadata": map[string]interface{}{
							"name":      "set-controller-reference",
							"namespace": "myclusterns",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "success no owner",
			fields: fields{
				client: nil,
				owner:  nil,
				scheme: testscheme,
				merger: nil,
			},
			args: args{
				u: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": corev1.SchemeGroupVersion.String(),
						"kind":       "ServiceAccount",
						"metadata": map[string]interface{}{
							"name":      "set-controller-reference",
							"namespace": "myclusterns",
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Applier{
				templateProcessor: tt.fields.templateProcessor,
				client:            tt.fields.client,
				owner:             tt.fields.owner,
				scheme:            tt.fields.scheme,
				merger:            tt.fields.merger,
			}
			err := a.setControllerReference(tt.args.u)
			if (err != nil) != tt.wantErr {
				t.Errorf("Applier.setControllerReference() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil {
				t.Log(tt.args.u.GetOwnerReferences())
				switch tt.name {
				case "success":
					if len(tt.args.u.GetOwnerReferences()) == 0 {
						t.Error("No ownerReference set")
					}
				case "success no owner":
					{
						if len(tt.args.u.GetOwnerReferences()) != 0 {
							t.Error("ownerReference found")
						}
					}
				}
			}
		})
	}
}

func TestApplier_CreateInPath(t *testing.T) {
	testscheme := scheme.Scheme

	testscheme.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.ClusterRole{})
	testscheme.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.ClusterRoleBinding{})
	testscheme.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.ServiceAccount{})

	tp, err := NewTemplateProcessor(NewTestReader(assets), nil)
	if err != nil {
		t.Errorf("Unable to create applier %s", err.Error())
	}

	client := fake.NewFakeClient([]runtime.Object{}...)

	a, err := NewApplier(tp, client, nil, nil, nil)
	if err != nil {
		t.Errorf("Unable to create applier %s", err.Error())
	}

	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      values.BootstrapServiceAccountName,
			Namespace: values.ManagedClusterNamespace,
		},
	}
	clientUpdate := fake.NewFakeClient(sa)

	aUpdate, err := NewApplier(tp, clientUpdate, nil, nil, DefaultKubernetesMerger)
	if err != nil {
		t.Errorf("Unable to create applier %s", err.Error())
	}
	type args struct {
		path      string
		excluded  []string
		recursive bool
		values    interface{}
	}
	tests := []struct {
		name    string
		fields  Applier
		args    args
		wantErr bool
	}{
		{
			name:   "success",
			fields: *a,
			args: args{
				path:      "test",
				excluded:  nil,
				recursive: false,
				values:    values,
			},
			wantErr: false,
		},
		{
			name:   "fail update",
			fields: *aUpdate,
			args: args{
				path:      "test",
				excluded:  nil,
				recursive: false,
				values:    values,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Applier{
				templateProcessor: tt.fields.templateProcessor,
				client:            tt.fields.client,
				owner:             tt.fields.owner,
				scheme:            tt.fields.scheme,
				merger:            tt.fields.merger,
			}
			if err := a.CreateInPath(tt.args.path, tt.args.excluded, tt.args.recursive, tt.args.values); (err != nil) != tt.wantErr {
				t.Errorf("Applier.CreateInPath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestApplier_UpdateInPath(t *testing.T) {
	testscheme := scheme.Scheme

	testscheme.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.ClusterRole{})
	testscheme.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.ClusterRoleBinding{})
	testscheme.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.ServiceAccount{})

	tp, err := NewTemplateProcessor(NewTestReader(assets), nil)
	if err != nil {
		t.Errorf("Unable to create applier %s", err.Error())
	}

	client := fake.NewFakeClient([]runtime.Object{}...)

	a, err := NewApplier(tp, client, nil, nil, nil)
	if err != nil {
		t.Errorf("Unable to create applier %s", err.Error())
	}

	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      values.BootstrapServiceAccountName,
			Namespace: values.ManagedClusterNamespace,
		},
	}
	saSecrets := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      values.BootstrapServiceAccountName,
			Namespace: values.ManagedClusterNamespace,
		},
		Secrets: []corev1.ObjectReference{
			{Name: "objectname"},
		},
	}

	clientUpdateNoMerger := fake.NewFakeClient(sa)

	aUpdateNoMerger, err := NewApplier(tp, clientUpdateNoMerger, nil, nil, nil)
	if err != nil {
		t.Errorf("Unable to create applier %s", err.Error())
	}

	clientUpdateMerged := fake.NewFakeClient(saSecrets)

	aUpdateMerged, err := NewApplier(tp, clientUpdateMerged, nil, nil, DefaultKubernetesMerger)
	if err != nil {
		t.Errorf("Unable to create applier %s", err.Error())
	}
	type args struct {
		path      string
		excluded  []string
		recursive bool
		values    interface{}
	}
	tests := []struct {
		name    string
		fields  Applier
		args    args
		wantErr bool
	}{
		{
			name:   "fail",
			fields: *a,
			args: args{
				path:      "test",
				excluded:  nil,
				recursive: false,
				values:    values,
			},
			wantErr: true,
		},
		{
			name:   "success update no merger",
			fields: *aUpdateNoMerger,
			args: args{
				path: "test",
				excluded: []string{
					"test/clusterrolebinding",
					"test/clusterrole",
				},
				recursive: false,
				values:    values,
			},
			wantErr: true,
		},
		{
			name:   "success update merged",
			fields: *aUpdateMerged,
			args: args{
				path: "test",
				excluded: []string{
					"test/clusterrolebinding",
					"test/clusterrole",
				},
				recursive: false,
				values:    values,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Applier{
				templateProcessor: tt.fields.templateProcessor,
				client:            tt.fields.client,
				owner:             tt.fields.owner,
				scheme:            tt.fields.scheme,
				merger:            tt.fields.merger,
			}
			if err := a.UpdateInPath(tt.args.path, tt.args.excluded, tt.args.recursive, tt.args.values); (err != nil) != tt.wantErr {
				t.Errorf("Applier.UpdateInPath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestApplier_CreateOrUpdateAssets(t *testing.T) {
	testscheme := scheme.Scheme

	testscheme.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.ClusterRole{})
	testscheme.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.ClusterRoleBinding{})
	testscheme.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.ServiceAccount{})

	tp, err := NewTemplateProcessor(NewTestReader(assets), nil)
	if err != nil {
		t.Errorf("Unable to create applier %s", err.Error())
	}

	client := fake.NewFakeClient([]runtime.Object{}...)

	a, err := NewApplier(tp, client, nil, nil, nil)
	if err != nil {
		t.Errorf("Unable to create applier %s", err.Error())
	}

	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      values.BootstrapServiceAccountName,
			Namespace: values.ManagedClusterNamespace,
		},
	}
	clientUpdate := fake.NewFakeClient(sa)

	aUpdate, err := NewApplier(tp, clientUpdate, nil, nil, DefaultKubernetesMerger)
	if err != nil {
		t.Errorf("Unable to create applier %s", err.Error())
	}

	clientUpdateNoMerger := fake.NewFakeClient(sa)

	aUpdateNoMerger, err := NewApplier(tp, clientUpdateNoMerger, nil, nil, nil)
	if err != nil {
		t.Errorf("Unable to create applier %s", err.Error())
	}

	type args struct {
		assets    []byte
		values    interface{}
		delimiter string
	}
	tests := []struct {
		name    string
		fields  Applier
		args    args
		wantErr bool
	}{
		{
			name:   "success",
			fields: *a,
			args: args{
				assets:    []byte(assetsYaml),
				values:    values,
				delimiter: "---",
			},
			wantErr: false,
		},
		{
			name:   "success update",
			fields: *aUpdate,
			args: args{
				assets:    []byte(assetsYaml),
				values:    values,
				delimiter: "---",
			},
			wantErr: false,
		},
		{
			name:   "success update no merger",
			fields: *aUpdateNoMerger,
			args: args{
				assets:    []byte(assetsYaml),
				values:    values,
				delimiter: "---",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Applier{
				templateProcessor: tt.fields.templateProcessor,
				client:            tt.fields.client,
				owner:             tt.fields.owner,
				scheme:            tt.fields.scheme,
				merger:            tt.fields.merger,
			}
			if err := a.CreateOrUpdateAssets(tt.args.assets, tt.args.values, tt.args.delimiter); (err != nil) != tt.wantErr {
				t.Errorf("Applier.CreateOrUpdateAssets() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
