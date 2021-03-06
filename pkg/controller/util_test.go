package controller

import (
	"fmt"
	"reflect"
	"testing"

	b64 "encoding/base64"

	"github.com/zalando/postgres-operator/pkg/spec"
	"github.com/zalando/postgres-operator/pkg/util/config"
	"github.com/zalando/postgres-operator/pkg/util/k8sutil"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	testInfrastructureRolesOldSecretName = "infrastructureroles-old-test"
	testInfrastructureRolesNewSecretName = "infrastructureroles-new-test"
)

func newUtilTestController() *Controller {
	controller := NewController(&spec.ControllerConfig{}, "util-test")
	controller.opConfig.ClusterNameLabel = "cluster-name"
	controller.opConfig.InfrastructureRolesSecretName =
		spec.NamespacedName{
			Namespace: v1.NamespaceDefault,
			Name:      testInfrastructureRolesOldSecretName,
		}
	controller.opConfig.Workers = 4
	controller.KubeClient = k8sutil.NewMockKubernetesClient()
	return controller
}

var utilTestController = newUtilTestController()

func TestPodClusterName(t *testing.T) {
	var testTable = []struct {
		in       *v1.Pod
		expected spec.NamespacedName
	}{
		{
			&v1.Pod{},
			spec.NamespacedName{},
		},
		{
			&v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: v1.NamespaceDefault,
					Labels: map[string]string{
						utilTestController.opConfig.ClusterNameLabel: "testcluster",
					},
				},
			},
			spec.NamespacedName{Namespace: v1.NamespaceDefault, Name: "testcluster"},
		},
	}
	for _, test := range testTable {
		resp := utilTestController.podClusterName(test.in)
		if resp != test.expected {
			t.Errorf("expected response %v does not match the actual %v", test.expected, resp)
		}
	}
}

func TestClusterWorkerID(t *testing.T) {
	var testTable = []struct {
		in       spec.NamespacedName
		expected uint32
	}{
		{
			in:       spec.NamespacedName{Namespace: "foo", Name: "bar"},
			expected: 0,
		},
		{
			in:       spec.NamespacedName{Namespace: "default", Name: "testcluster"},
			expected: 1,
		},
	}
	for _, test := range testTable {
		resp := utilTestController.clusterWorkerID(test.in)
		if resp != test.expected {
			t.Errorf("expected response %v does not match the actual %v", test.expected, resp)
		}
	}
}

// Test functionality of getting infrastructure roles from their description in
// corresponding secrets. Here we test only common stuff (e.g. when a secret do
// not exist, or empty) and the old format.
func TestOldInfrastructureRoleFormat(t *testing.T) {
	var testTable = []struct {
		secretName     spec.NamespacedName
		expectedRoles  map[string]spec.PgUser
		expectedErrors []error
	}{
		{
			// empty secret name
			spec.NamespacedName{},
			nil,
			nil,
		},
		{
			// secret does not exist
			spec.NamespacedName{Namespace: v1.NamespaceDefault, Name: "null"},
			map[string]spec.PgUser{},
			[]error{fmt.Errorf(`could not get infrastructure roles secret default/null: NotFound`)},
		},
		{
			spec.NamespacedName{
				Namespace: v1.NamespaceDefault,
				Name:      testInfrastructureRolesOldSecretName,
			},
			map[string]spec.PgUser{
				"testrole": {
					Name:     "testrole",
					Origin:   spec.RoleOriginInfrastructure,
					Password: "testpassword",
					MemberOf: []string{"testinrole"},
				},
				"foobar": {
					Name:     "foobar",
					Origin:   spec.RoleOriginInfrastructure,
					Password: b64.StdEncoding.EncodeToString([]byte("password")),
					MemberOf: nil,
				},
			},
			nil,
		},
	}
	for _, test := range testTable {
		roles, errors := utilTestController.getInfrastructureRoles(
			[]*config.InfrastructureRole{
				&config.InfrastructureRole{
					SecretName:  test.secretName,
					UserKey:     "user",
					PasswordKey: "password",
					RoleKey:     "inrole",
					Template:    true,
				},
			})

		if len(errors) != len(test.expectedErrors) {
			t.Errorf("expected error '%v' does not match the actual error '%v'",
				test.expectedErrors, errors)
		}

		for idx := range errors {
			err := errors[idx]
			expectedErr := test.expectedErrors[idx]

			if err != expectedErr {
				if err != nil && expectedErr != nil && err.Error() == expectedErr.Error() {
					continue
				}
				t.Errorf("expected error '%v' does not match the actual error '%v'",
					expectedErr, err)
			}
		}

		if !reflect.DeepEqual(roles, test.expectedRoles) {
			t.Errorf("expected roles output %#v does not match the actual %#v",
				test.expectedRoles, roles)
		}
	}
}

// Test functionality of getting infrastructure roles from their description in
// corresponding secrets. Here we test the new format.
func TestNewInfrastructureRoleFormat(t *testing.T) {
	var testTable = []struct {
		secrets        []spec.NamespacedName
		expectedRoles  map[string]spec.PgUser
		expectedErrors []error
	}{
		// one secret with one configmap
		{
			[]spec.NamespacedName{
				spec.NamespacedName{
					Namespace: v1.NamespaceDefault,
					Name:      testInfrastructureRolesNewSecretName,
				},
			},
			map[string]spec.PgUser{
				"new-test-role": {
					Name:     "new-test-role",
					Origin:   spec.RoleOriginInfrastructure,
					Password: "new-test-password",
					MemberOf: []string{"new-test-inrole"},
				},
				"new-foobar": {
					Name:     "new-foobar",
					Origin:   spec.RoleOriginInfrastructure,
					Password: b64.StdEncoding.EncodeToString([]byte("password")),
					MemberOf: nil,
					Flags:    []string{"createdb"},
				},
			},
			nil,
		},
		// multiple standalone secrets
		{
			[]spec.NamespacedName{
				spec.NamespacedName{
					Namespace: v1.NamespaceDefault,
					Name:      "infrastructureroles-new-test1",
				},
				spec.NamespacedName{
					Namespace: v1.NamespaceDefault,
					Name:      "infrastructureroles-new-test2",
				},
			},
			map[string]spec.PgUser{
				"new-test-role1": {
					Name:     "new-test-role1",
					Origin:   spec.RoleOriginInfrastructure,
					Password: "new-test-password1",
					MemberOf: []string{"new-test-inrole1"},
				},
				"new-test-role2": {
					Name:     "new-test-role2",
					Origin:   spec.RoleOriginInfrastructure,
					Password: "new-test-password2",
					MemberOf: []string{"new-test-inrole2"},
				},
			},
			nil,
		},
	}
	for _, test := range testTable {
		definitions := []*config.InfrastructureRole{}
		for _, secret := range test.secrets {
			definitions = append(definitions, &config.InfrastructureRole{
				SecretName:  secret,
				UserKey:     "user",
				PasswordKey: "password",
				RoleKey:     "inrole",
				Template:    false,
			})
		}

		roles, errors := utilTestController.getInfrastructureRoles(definitions)
		if len(errors) != len(test.expectedErrors) {
			t.Errorf("expected error does not match the actual error:\n%+v\n%+v",
				test.expectedErrors, errors)

			// Stop and do not do any further checks
			return
		}

		for idx := range errors {
			err := errors[idx]
			expectedErr := test.expectedErrors[idx]

			if err != expectedErr {
				if err != nil && expectedErr != nil && err.Error() == expectedErr.Error() {
					continue
				}
				t.Errorf("expected error '%v' does not match the actual error '%v'",
					expectedErr, err)
			}
		}

		if !reflect.DeepEqual(roles, test.expectedRoles) {
			t.Errorf("expected roles output/the actual:\n%#v\n%#v",
				test.expectedRoles, roles)
		}
	}
}

// Tests for getting correct infrastructure roles definitions from present
// configuration. E.g. in which secrets for which roles too look. The biggest
// point here is compatibility of old and new formats of defining
// infrastructure roles.
func TestInfrastructureRoleDefinitions(t *testing.T) {
	var testTable = []struct {
		rolesDefs      []*config.InfrastructureRole
		roleSecretName spec.NamespacedName
		roleSecrets    string
		expectedDefs   []*config.InfrastructureRole
	}{
		// only new format
		{
			[]*config.InfrastructureRole{
				&config.InfrastructureRole{
					SecretName: spec.NamespacedName{
						Namespace: v1.NamespaceDefault,
						Name:      testInfrastructureRolesNewSecretName,
					},
					UserKey:     "user",
					PasswordKey: "password",
					RoleKey:     "inrole",
					Template:    false,
				},
			},
			spec.NamespacedName{},
			"",
			[]*config.InfrastructureRole{
				&config.InfrastructureRole{
					SecretName: spec.NamespacedName{
						Namespace: v1.NamespaceDefault,
						Name:      testInfrastructureRolesNewSecretName,
					},
					UserKey:     "user",
					PasswordKey: "password",
					RoleKey:     "inrole",
					Template:    false,
				},
			},
		},
		// only old format
		{
			[]*config.InfrastructureRole{},
			spec.NamespacedName{
				Namespace: v1.NamespaceDefault,
				Name:      testInfrastructureRolesOldSecretName,
			},
			"",
			[]*config.InfrastructureRole{
				&config.InfrastructureRole{
					SecretName: spec.NamespacedName{
						Namespace: v1.NamespaceDefault,
						Name:      testInfrastructureRolesOldSecretName,
					},
					UserKey:     "user",
					PasswordKey: "password",
					RoleKey:     "inrole",
					Template:    true,
				},
			},
		},
		// only configmap format
		{
			[]*config.InfrastructureRole{},
			spec.NamespacedName{
				Namespace: v1.NamespaceDefault,
				Name:      testInfrastructureRolesOldSecretName,
			},
			"secretname: infrastructureroles-old-test, userkey: test-user, passwordkey: test-password, rolekey: test-role, template: false",
			[]*config.InfrastructureRole{
				&config.InfrastructureRole{
					SecretName: spec.NamespacedName{
						Namespace: v1.NamespaceDefault,
						Name:      testInfrastructureRolesOldSecretName,
					},
					UserKey:     "test-user",
					PasswordKey: "test-password",
					RoleKey:     "test-role",
					Template:    false,
				},
			},
		},
		// incorrect configmap format
		{
			[]*config.InfrastructureRole{},
			spec.NamespacedName{
				Namespace: v1.NamespaceDefault,
				Name:      testInfrastructureRolesOldSecretName,
			},
			"wrong-format",
			[]*config.InfrastructureRole{},
		},
		// configmap without a secret
		{
			[]*config.InfrastructureRole{},
			spec.NamespacedName{},
			"userkey: test-user, passwordkey: test-password, rolekey: test-role, template: false",
			[]*config.InfrastructureRole{},
		},
	}

	for _, test := range testTable {
		t.Logf("Test: %+v", test)
		utilTestController.opConfig.InfrastructureRoles = test.rolesDefs
		utilTestController.opConfig.InfrastructureRolesSecretName = test.roleSecretName
		utilTestController.opConfig.InfrastructureRolesDefs = test.roleSecrets

		defs := utilTestController.getInfrastructureRoleDefinitions()
		if len(defs) != len(test.expectedDefs) {
			t.Errorf("expected definitions does not match the actual:\n%#v\n%#v",
				test.expectedDefs, defs)

			// Stop and do not do any further checks
			return
		}

		for idx := range defs {
			def := defs[idx]
			expectedDef := test.expectedDefs[idx]

			if !reflect.DeepEqual(def, expectedDef) {
				t.Errorf("expected definition/the actual:\n%#v\n%#v",
					expectedDef, def)
			}
		}
	}
}
