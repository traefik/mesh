package controller

import (
	"os"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
)

func TestEnqueueWorkHandler_OnAdd(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stdout)
	logger.SetLevel(logrus.DebugLevel)

	workQueue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	handler := &enqueueWorkHandler{logger: logger, workQueue: workQueue}
	handler.OnAdd(&corev1.Pod{})

	assert.Equal(t, 1, workQueue.Len())

	currentKey, _ := workQueue.Get()

	assert.Equal(t, configRefreshKey, currentKey)
}

func TestEnqueueWorkHandler_OnDelete(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stdout)
	logger.SetLevel(logrus.DebugLevel)

	workQueue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	handler := &enqueueWorkHandler{logger: logger, workQueue: workQueue}
	handler.OnDelete(&corev1.Pod{})

	assert.Equal(t, 1, workQueue.Len())

	currentKey, _ := workQueue.Get()

	assert.Equal(t, configRefreshKey, currentKey)
}

func TestEnqueueWorkHandler_OnUpdate(t *testing.T) {
	tests := []struct {
		desc        string
		oldObj      interface{}
		newObj      interface{}
		expectedLen int
	}{
		{
			desc: "should not enqueue if this is a re-sync event",
			oldObj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{ResourceVersion: "foo"},
			},
			newObj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{ResourceVersion: "foo"},
			},
			expectedLen: 0,
		},
		{
			desc: "should enqueue if this is not a re-sync event",
			oldObj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{ResourceVersion: "foo"},
			},
			newObj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{ResourceVersion: "bar"},
			},
			expectedLen: 1,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			logger := logrus.New()
			logger.SetOutput(os.Stdout)
			logger.SetLevel(logrus.DebugLevel)

			workQueue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

			handler := &enqueueWorkHandler{logger: logger, workQueue: workQueue}
			handler.OnUpdate(test.oldObj, test.newObj)

			assert.Equal(t, test.expectedLen, workQueue.Len())
		})
	}
}

func TestEnqueueWorkHandler_enqueueWork(t *testing.T) {
	tests := []struct {
		desc        string
		obj         interface{}
		expectedLen int
		expectedKey string
	}{
		{
			desc:        "should enqueue a refresh key if obj is not a service",
			obj:         &corev1.Endpoints{},
			expectedLen: 1,
			expectedKey: configRefreshKey,
		},
		{
			desc: "should enqueue a meta namespace key if the obj is a service",
			obj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
			},
			expectedLen: 1,
			expectedKey: "bar/foo",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			logger := logrus.New()
			logger.SetOutput(os.Stdout)
			logger.SetLevel(logrus.DebugLevel)

			workQueue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

			handler := &enqueueWorkHandler{logger: logger, workQueue: workQueue}
			handler.enqueueWork(test.obj)

			assert.Equal(t, test.expectedLen, workQueue.Len())

			currentKey, _ := workQueue.Get()

			assert.Equal(t, test.expectedKey, currentKey)
		})
	}
}
