package service

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"sync"

	"github.com/google/uuid"
	jsoniter "github.com/json-iterator/go"
	"github.com/openshift-kni/oran-o2ims/internal/data"
	"github.com/openshift-kni/oran-o2ims/internal/jq"
	"github.com/openshift-kni/oran-o2ims/internal/k8s"
	"github.com/openshift-kni/oran-o2ims/internal/persiststorage"
	"github.com/openshift-kni/oran-o2ims/internal/search"
)

// InfrastructureInventorySubscriptionHandlerBuilder contains the data and logic needed to create a new
// infrastructure inventory subscription collection handler.
// Don't create instances of this type directly, use the NewInfrastructureInventorySubscriptionHandler
// function instead.
type InfrastructureInventorySubscriptionHandlerBuilder struct {
	logger         *slog.Logger
	loggingWrapper func(http.RoundTripper) http.RoundTripper
	cloudID        string
	extensions     []string
	kubeClient     *k8s.Client
}

// InfrastructureInventorySubscriptionHandler knows how to respond to requests to list resources. Don't create
// instances of this type directly, use the NewResourceHandler function instead.
type InfrastructureInventorySubscriptionHandler struct {
	logger                    *slog.Logger
	loggingWrapper            func(http.RoundTripper) http.RoundTripper
	cloudID                   string
	extensions                []string
	kubeClient                *k8s.Client
	jsonAPI                   jsoniter.API
	selectorEvaluator         *search.SelectorEvaluator
	jqTool                    *jq.Tool
	subscriptionMapMemoryLock *sync.Mutex
	subscriptionMap           *map[string]data.Object
	persistStore              *persiststorage.KubeConfigMapStore
}

// InfrastructureInventorySubscriptionHandler creates a builder that can then be used to configure and create a
// handler for the collection of InfrastructureInventorySubscriptions.
func NewInfrastructureInventorySubscriptionHandler() *InfrastructureInventorySubscriptionHandlerBuilder {
	return &InfrastructureInventorySubscriptionHandlerBuilder{}
}

// SetLogger sets the logger that the handler will use to write to the log. This is mandatory.
func (b *InfrastructureInventorySubscriptionHandlerBuilder) SetLogger(
	value *slog.Logger) *InfrastructureInventorySubscriptionHandlerBuilder {
	b.logger = value
	return b
}

// SetLoggingWrapper sets the wrapper that will be used to configure logging for the HTTP clients
// used to connect to other servers, including the backend server. This is optional.
func (b *InfrastructureInventorySubscriptionHandlerBuilder) SetLoggingWrapper(
	value func(http.RoundTripper) http.RoundTripper) *InfrastructureInventorySubscriptionHandlerBuilder {
	b.loggingWrapper = value
	return b
}

// SetCloudID sets the identifier of the O-Cloud of this handler. This is mandatory.
func (b *InfrastructureInventorySubscriptionHandlerBuilder) SetCloudID(
	value string) *InfrastructureInventorySubscriptionHandlerBuilder {
	b.cloudID = value
	return b
}

// SetExtensions sets the fields that will be added to the extensions.
func (b *InfrastructureInventorySubscriptionHandlerBuilder) SetExtensions(
	values ...string) *InfrastructureInventorySubscriptionHandlerBuilder {
	b.extensions = values
	return b
}

// SetKubeClient sets the K8S client.
func (b *InfrastructureInventorySubscriptionHandlerBuilder) SetKubeClient(
	kubeClient *k8s.Client) *InfrastructureInventorySubscriptionHandlerBuilder {
	b.kubeClient = kubeClient
	return b
}

// Build uses the data stored in the builder to create and configure a new handler.
func (b *InfrastructureInventorySubscriptionHandlerBuilder) Build(ctx context.Context) (
	result *InfrastructureInventorySubscriptionHandler, err error) {
	// Check parameters:
	if b.logger == nil {
		err = errors.New("logger is mandatory")
		return
	}
	if b.cloudID == "" {
		err = errors.New("cloud identifier is mandatory")
		return
	}

	if b.kubeClient == nil {
		err = errors.New("kubeClient is mandatory")
		return
	}

	// Prepare the JSON iterator API:
	jsonConfig := jsoniter.Config{
		IndentionStep: 2,
	}
	jsonAPI := jsonConfig.Froze()

	// Create the filter expression evaluator:
	pathEvaluator, err := search.NewPathEvaluator().
		SetLogger(b.logger).
		Build()
	if err != nil {
		return
	}
	selectorEvaluator, err := search.NewSelectorEvaluator().
		SetLogger(b.logger).
		SetPathEvaluator(pathEvaluator.Evaluate).
		Build()
	if err != nil {
		return
	}

	// Create the jq tool:
	jqTool, err := jq.NewTool().
		SetLogger(b.logger).
		Build()
	if err != nil {
		return
	}

	// Check that extensions are at least syntactically valid:
	for _, extension := range b.extensions {
		_, err = jqTool.Compile(extension)
		if err != nil {
			return
		}
	}

	// Create the persistent storage ConfigMap:
	persistStore := persiststorage.NewKubeConfigMapStore().
		SetNameSpace(TestNamespace).
		SetName(TestConfigmapName).
		SetFieldOwner(FieldOwner).
		SetJsonAPI(&jsonAPI).
		SetClient(b.kubeClient)

	// Create and populate the object:
	result = &InfrastructureInventorySubscriptionHandler{
		logger:                    b.logger,
		loggingWrapper:            b.loggingWrapper,
		cloudID:                   b.cloudID,
		kubeClient:                b.kubeClient,
		extensions:                slices.Clone(b.extensions),
		selectorEvaluator:         selectorEvaluator,
		jsonAPI:                   jsonAPI,
		jqTool:                    jqTool,
		subscriptionMapMemoryLock: &sync.Mutex{},
		subscriptionMap:           &map[string]data.Object{},
		persistStore:              persistStore,
	}

	b.logger.Debug(
		"InfrastructureInventorySubscriptionHandler build:",
		"CloudID", b.cloudID,
	)

	err = result.getFromPersistentStorage(ctx)
	if err != nil {
		b.logger.Error(
			"infrastructureInventorySubscriptionHandler failed to retrieve from persistStore ", err,
		)
	}

	err = result.watchPersistStore(ctx)
	if err != nil {
		b.logger.Error(
			"infrastructureInventorySubscriptionHandler failed to watch persist store changes ", err,
		)
	}

	return
}

func (h *InfrastructureInventorySubscriptionHandler) fetchItems(ctx context.Context) (result data.Stream, err error) {

	h.subscriptionMapMemoryLock.Lock()
	defer h.subscriptionMapMemoryLock.Unlock()

	ar := make([]data.Object, 0, len(*h.subscriptionMap))

	for key, value := range *h.subscriptionMap {
		obj, _ := h.encodeSubId(ctx, key, value)
		ar = append(ar, obj)
	}
	h.logger.DebugContext(
		ctx,
		"InfrastructureInventorySubscriptionHandler fetchItems:",
	)
	result = data.Pour(ar...)

	return
}

func (h *InfrastructureInventorySubscriptionHandler) mapItem(ctx context.Context,
	input data.Object) (output data.Object, err error) {

	//TBD only save related attributes in the future
	return input, nil
}

func (h *InfrastructureInventorySubscriptionHandler) encodeSubId(ctx context.Context,
	subId string, input data.Object) (output data.Object, err error) {
	//get consumer name, subscriptions
	err = h.jqTool.Evaluate(
		`{
			"subscriptionId": $subId,
			"consumerSubscriptionId": .consumerSubscriptionId,
			"callback": .callback,
			"filter": .filter
		}`,
		input, &output,
		jq.String("$subId", subId),
	)
	if err != nil {
		return
	}
	return
}

// List is the implementation of the collection handler interface.
func (h *InfrastructureInventorySubscriptionHandler) List(ctx context.Context,
	request *ListRequest) (response *ListResponse, err error) {
	// Create the stream that will fetch the items:
	var items data.Stream

	items, err = h.fetchItems(ctx)

	if err != nil {
		return
	}

	// Transform the items into what we need:
	items = data.Map(items, h.mapItem)

	// Select only the items that satisfy the filter:
	if request.Selector != nil {
		items = data.Select(
			items,
			func(ctx context.Context, item data.Object) (result bool, err error) {
				result, err = h.selectorEvaluator.Evaluate(ctx, request.Selector, item)
				return
			},
		)
	}

	// Return the result:
	response = &ListResponse{
		Items: items,
	}
	return
}

// Get is the implementation of the object handler interface.
func (h *InfrastructureInventorySubscriptionHandler) Get(ctx context.Context,
	request *GetRequest) (response *GetResponse, err error) {

	h.logger.DebugContext(
		ctx,
		"infrastructureInventorySubscriptionHandler Get:",
	)
	item, err := h.fetchItem(ctx, request.Variables[0])

	// Return the result:
	response = &GetResponse{
		Object: item,
	}
	return
}

// Add is the implementation of the object handler ADD interface.
func (h *InfrastructureInventorySubscriptionHandler) Add(ctx context.Context,
	request *AddRequest) (response *AddResponse, err error) {

	h.logger.DebugContext(
		ctx,
		"infrastructureInventorySubscriptionHandler Add:",
	)
	id, err := h.addItem(ctx, *request)

	if err != nil {
		h.logger.Debug(
			"infrastructureInventorySubscriptionHandler Add:",
			"err", err.Error(),
		)
		return
	}

	//add subscription Id in the response
	obj := request.Object

	obj, err = h.encodeSubId(ctx, id, obj)

	if err != nil {
		return
	}

	// Return the result:
	response = &AddResponse{
		Object: obj,
	}
	return
}

// Delete is the implementation of the object handler delete interface.
func (h *InfrastructureInventorySubscriptionHandler) Delete(ctx context.Context,
	request *DeleteRequest) (response *DeleteResponse, err error) {

	h.logger.DebugContext(
		ctx,
		"infrastructureInventorySubscriptionHandler delete:",
	)

	err = h.deleteItem(ctx, *request)

	// Return the result:
	response = &DeleteResponse{}

	return
}

func (h *InfrastructureInventorySubscriptionHandler) getFromPersistentStorage(ctx context.Context) (err error) {
	newMap, err := persiststorage.GetAll(h.persistStore, ctx)
	if err != nil {
		return
	}
	h.assignSubscriptionMap(newMap)
	return
}

func (h *InfrastructureInventorySubscriptionHandler) watchPersistStore(ctx context.Context) (err error) {
	err = persiststorage.ProcessChanges(h.persistStore, ctx, &h.subscriptionMap, h.subscriptionMapMemoryLock)

	if err != nil {
		panic("failed to launch watcher")
	}
	return
}

func (h *InfrastructureInventorySubscriptionHandler) assignSubscriptionMap(newMap map[string]data.Object) {
	h.subscriptionMapMemoryLock.Lock()
	defer h.subscriptionMapMemoryLock.Unlock()
	h.subscriptionMap = &newMap
}
func (h *InfrastructureInventorySubscriptionHandler) fetchItem(ctx context.Context,
	id string) (result data.Object, err error) {
	h.subscriptionMapMemoryLock.Lock()
	defer h.subscriptionMapMemoryLock.Unlock()
	obj, ok := (*h.subscriptionMap)[id]
	if !ok {
		err = ErrNotFound
		return
	}

	result, _ = h.encodeSubId(ctx, id, obj)
	return
}

func (h *InfrastructureInventorySubscriptionHandler) addItem(
	ctx context.Context, input_data AddRequest) (subId string, err error) {

	subId = h.getSubcriptionId()

	//save the subscription in configuration map
	//value, err := jsoniter.MarshalIndent(&input_data.Object, "", " ")
	value, err := h.jsonAPI.MarshalIndent(&input_data.Object, "", " ")
	if err != nil {
		return
	}
	err = h.persistStoreAddEntry(ctx, subId, string(value))
	if err != nil {
		h.logger.Debug(
			"infrastructureInventorySubscriptionHandler addItem:",
			"err", err.Error(),
		)
		return
	}

	h.addToSubscriptionMap(subId, input_data.Object)

	return
}

func (h *InfrastructureInventorySubscriptionHandler) deleteItem(
	ctx context.Context, delete_req DeleteRequest) (err error) {

	err = h.persistStoreDeleteEntry(ctx, delete_req.Variables[0])
	if err != nil {
		return
	}

	h.deleteToSubscriptionMap(delete_req.Variables[0])

	return
}

func (h *InfrastructureInventorySubscriptionHandler) addToSubscriptionMap(key string, value data.Object) {
	h.subscriptionMapMemoryLock.Lock()
	defer h.subscriptionMapMemoryLock.Unlock()
	(*h.subscriptionMap)[key] = value
}
func (h *InfrastructureInventorySubscriptionHandler) deleteToSubscriptionMap(key string) {
	h.subscriptionMapMemoryLock.Lock()
	defer h.subscriptionMapMemoryLock.Unlock()
	//test if the key in the map
	_, ok := (*h.subscriptionMap)[key]

	if !ok {
		return
	}

	delete(*h.subscriptionMap, key)
}

func (h *InfrastructureInventorySubscriptionHandler) getSubcriptionId() (subId string) {
	subId = uuid.New().String()
	return
}

func (h *InfrastructureInventorySubscriptionHandler) decodeSubId(ctx context.Context,
	input data.Object) (output string, err error) {
	//get cluster name, subscriptions
	err = h.jqTool.Evaluate(
		`.infrastructureInventorySubscriptionId`, input, &output)
	if err != nil {
		return
	}
	return
}

func (h *InfrastructureInventorySubscriptionHandler) persistStoreAddEntry(
	ctx context.Context, entryKey string, value string) (err error) {
	return persiststorage.Add(h.persistStore, ctx, entryKey, value)
}

func (h *InfrastructureInventorySubscriptionHandler) persistStoreDeleteEntry(
	ctx context.Context, entryKey string) (err error) {
	err = persiststorage.Delete(h.persistStore, ctx, entryKey)
	return
}

