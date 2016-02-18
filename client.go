package betwixt

import (
	"errors"
	. "github.com/zubairhamed/canopus"
	"log"
)

// NewDefaultClient instantiates a new instance of LWM2M Client
func NewDefaultClient(local string, remote string, registry Registry) *DefaultClient {
	coapServer := NewServer(local, remote)

	// Create Mandatory
	c := &DefaultClient{
		coapServer:     coapServer,
		enabledObjects: make(map[LWM2MObjectType]Object),
		registry:       registry,
	}

	mandatory := registry.GetMandatory()
	for _, o := range mandatory {
		c.EnableObject(o.GetType(), NewNullEnabler())
	}

	return c
}

type DefaultClient struct {
	coapServer     CoapServer
	registry       Registry
	enabledObjects map[LWM2MObjectType]Object
	path           string

	// Events
	evtOnStartup      FnOnStartup
	evtOnRead         FnOnRead
	evtOnWrite        FnOnWrite
	evtOnExecute      FnOnExecute
	evtOnRegistered   FnOnRegistered
	evtOnDeregistered FnOnDeregistered
	evtOnError        FnOnError
}

// Registers this client to a LWM2M Server instance
// name must be unique and be less than 10 characers
func (c *DefaultClient) Register(name string) (string, error) {
	if len(name) > 10 {
		return "", errors.New("Client name can not exceed 10 characters")
	}

	req := NewRequest(MessageConfirmable, Post, GenerateMessageID())

	req.SetStringPayload(BuildModelResourceStringPayload(c.enabledObjects))
	req.SetRequestURI("/rd")
	req.SetURIQuery("ep", name)
	resp, err := c.coapServer.Send(req)

	path := ""
	if err != nil {
		return "", err
	} else {
		path = resp.GetMessage().GetLocationPath()
	}
	c.path = path

	PrintMessage(resp.GetMessage())

	return path, nil
}

// Sets/Defines an Enabler for a given LWM2M Object Type
func (c *DefaultClient) SetEnabler(t LWM2MObjectType, e ObjectEnabler) {
	_, ok := c.enabledObjects[t]
	if ok {
		c.enabledObjects[t].SetEnabler(e)
	}
}

// Returns a list of LWM2M Enabled Objects
func (c *DefaultClient) GetEnabledObjects() map[LWM2MObjectType]Object {
	return c.enabledObjects
}

// Returns the registry used for looking up LWM2M object type definitions
func (c *DefaultClient) GetRegistry() Registry {
	return c.registry
}

// Unregisters this client from a LWM2M server which was previously registered
func (c *DefaultClient) Deregister() {
	req := NewRequest(MessageConfirmable, Delete, GenerateMessageID())

	req.SetRequestURI(c.path)
	_, err := c.coapServer.Send(req)

	if err != nil {
		log.Println(err)
	}
}

func (c *DefaultClient) Update() {

}

func (c *DefaultClient) AddResource() {

}

func (c *DefaultClient) AddObject() {

}

func (c *DefaultClient) UseRegistry(reg Registry) {
	c.registry = reg
}

// Registes an object enabler for a given LWM2M object type
func (c *DefaultClient) EnableObject(t LWM2MObjectType, e ObjectEnabler) error {
	_, ok := c.enabledObjects[t]
	if !ok {
		if c.registry == nil {
			return errors.New("No registry found/set")
		}
		c.enabledObjects[t] = NewObject(t, e, c.registry)

		return nil
	} else {
		return errors.New("Object already enabled")
	}
}

// Adds a new object instance for a previously enabled LWM2M object type
func (c *DefaultClient) AddObjectInstance(t LWM2MObjectType, instance int) error {
	o := c.enabledObjects[t]
	if o != nil {
		o.AddInstance(instance)

		return nil
	}
	return errors.New("Attempting to add a nil instance")
}

// Adds a list of object instance for a previously enabled LWM2M object type
func (c *DefaultClient) AddObjectInstances(t LWM2MObjectType, instances ...int) {
	for _, o := range instances {
		c.AddObjectInstance(t, o)
	}
}

func (c *DefaultClient) GetObject(n LWM2MObjectType) Object {
	return c.enabledObjects[n]
}

func (c *DefaultClient) validate() {

}

// Starts up the LWM2M client, listens to incoming requests and fires the OnStart event
func (c *DefaultClient) Start() {
	c.validate()

	s := c.coapServer
	s.OnStart(func(server CoapServer) {
		if c.evtOnStartup != nil {
			c.evtOnStartup()
		}
	})

	s.OnObserve(func(resource string, msg *Message) {
		log.Println("Observe Requested")
	})

	s.Get("/:obj/:inst/:rsrc", c.handleReadRequest)
	s.Get("/:obj/:inst", c.handleReadRequest)
	s.Get("/:obj", c.handleReadRequest)

	s.Put("/:obj/:inst/:rsrc", c.handleWriteRequest)
	s.Put("/:obj/:inst", c.handleWriteRequest)

	s.Delete("/:obj/:inst", c.handleDeleteRequest)

	s.Post("/:obj/:inst/:rsrc", c.handleExecuteRequest)
	s.Post("/:obj/:inst", c.handleCreateRequest)

	c.coapServer.Start()
}

// Handles LWM2M Create Requests (not to be mistaken for/not the same as  CoAP PUT)
func (c *DefaultClient) handleCreateRequest(req CoapRequest) CoapResponse {
	log.Println("Create Request")
	attrResource := req.GetAttribute("rsrc")
	objectId := req.GetAttributeAsInt("obj")
	instanceId := req.GetAttributeAsInt("inst")

	var resourceId = -1

	if attrResource != "" {
		resourceId = req.GetAttributeAsInt("rsrc")
	}

	t := LWM2MObjectType(objectId)
	obj := c.GetObject(t)
	enabler := obj.GetEnabler()

	msg := NewMessageOfType(MessageAcknowledgment, req.GetMessage().MessageID)
	msg.Token = req.GetMessage().Token
	msg.Payload = NewEmptyPayload()

	if enabler != nil {
		lwReq := Default(req, OPERATIONTYPE_CREATE)
		response := enabler.OnCreate(instanceId, resourceId, lwReq)
		msg.Code = response.GetResponseCode()
	} else {
		msg.Code = CoapCodeMethodNotAllowed
	}
	return NewResponseWithMessage(msg)
}

// Handles LWM2M Read Requests (not to be mistaken for/not the same as  CoAP GET)
func (c *DefaultClient) handleReadRequest(req CoapRequest) CoapResponse {
	log.Println("Read Request")
	attrResource := req.GetAttribute("rsrc")
	objectId := req.GetAttributeAsInt("obj")
	instanceId := req.GetAttributeAsInt("inst")

	var resourceId = -1

	if attrResource != "" {
		resourceId = req.GetAttributeAsInt("rsrc")
	}

	t := LWM2MObjectType(objectId)
	obj := c.GetObject(t)
	enabler := obj.GetEnabler()

	msg := NewMessageOfType(MessageAcknowledgment, req.GetMessage().MessageID)
	msg.Token = req.GetMessage().Token

	if enabler != nil {
		model := obj.GetDefinition()
		resource := model.GetResource(uint16(resourceId))

		if resource == nil {
			// TODO: Return TLV of Object Instance
			msg.Code = CoapCodeNotFound
		} else {
			if !IsReadableResource(resource) {
				msg.Code = CoapCodeMethodNotAllowed
			} else {
				lwReq := Default(req, OPERATIONTYPE_READ)
				response := enabler.OnRead(instanceId, resourceId, lwReq)

				val := response.GetResponseValue()
				msg.Code = response.GetResponseCode()
				b := EncodeValue(resource.GetId(), resource.MultipleValuesAllowed(), val)
				msg.Payload = NewBytesPayload(b)
			}
		}
	} else {
		msg.Code = CoapCodeMethodNotAllowed
	}
	return NewResponseWithMessage(msg)
}

// Handles LWM2M Delete Requests (not to be mistaken for/not the same as  CoAP DELETE)
func (c *DefaultClient) handleDeleteRequest(req CoapRequest) CoapResponse {
	log.Println("Delete Request")
	objectId := req.GetAttributeAsInt("obj")
	instanceId := req.GetAttributeAsInt("inst")

	t := LWM2MObjectType(objectId)
	enabler := c.GetObject(t).GetEnabler()

	msg := NewMessageOfType(MessageAcknowledgment, req.GetMessage().MessageID)
	msg.Token = req.GetMessage().Token
	msg.Payload = NewEmptyPayload()

	if enabler != nil {
		lwReq := Default(req, OPERATIONTYPE_DELETE)

		response := enabler.OnDelete(instanceId, lwReq)
		msg.Code = response.GetResponseCode()
	} else {
		msg.Code = CoapCodeMethodNotAllowed
	}
	return NewResponseWithMessage(msg)
}

func (c *DefaultClient) handleDiscoverRequest() {
	log.Println("Discovery Request")
}

func (c *DefaultClient) handleObserveRequest() {
	log.Println("Observe Request")
}

// Handles LWM2M Write Requests (not to be mistaken for/not the same as  CoAP POST)
func (c *DefaultClient) handleWriteRequest(req CoapRequest) CoapResponse {
	log.Println("Write Request")
	attrResource := req.GetAttribute("rsrc")
	objectId := req.GetAttributeAsInt("obj")
	instanceId := req.GetAttributeAsInt("inst")

	var resourceId = -1

	if attrResource != "" {
		resourceId = req.GetAttributeAsInt("rsrc")
	}

	t := LWM2MObjectType(objectId)
	obj := c.GetObject(t)
	enabler := obj.GetEnabler()

	msg := NewMessageOfType(MessageAcknowledgment, req.GetMessage().MessageID)
	msg.Token = req.GetMessage().Token
	msg.Payload = NewEmptyPayload()

	if enabler != nil {
		model := obj.GetDefinition()
		resource := model.GetResource(uint16(resourceId))
		if resource == nil {
			// TODO Write to Object Instance
			msg.Code = CoapCodeNotFound
		} else {
			if !IsWritableResource(resource) {
				msg.Code = CoapCodeMethodNotAllowed
			} else {
				lwReq := Default(req, OPERATIONTYPE_WRITE)
				response := enabler.OnWrite(instanceId, resourceId, lwReq)
				msg.Code = response.GetResponseCode()
			}
		}
	} else {
		msg.Code = CoapCodeNotFound
	}
	return NewResponseWithMessage(msg)
}

// Handles LWM2M Execute Requests
func (c *DefaultClient) handleExecuteRequest(req CoapRequest) CoapResponse {
	log.Println("Execute Request")
	attrResource := req.GetAttribute("rsrc")
	objectId := req.GetAttributeAsInt("obj")
	instanceId := req.GetAttributeAsInt("inst")

	var resourceId = -1

	if attrResource != "" {
		resourceId = req.GetAttributeAsInt("rsrc")
	}

	t := LWM2MObjectType(objectId)
	obj := c.GetObject(t)
	enabler := obj.GetEnabler()

	msg := NewMessageOfType(MessageAcknowledgment, req.GetMessage().MessageID)
	msg.Token = req.GetMessage().Token
	msg.Payload = NewEmptyPayload()

	if enabler != nil {
		model := obj.GetDefinition()
		resource := model.GetResource(uint16(resourceId))
		if resource == nil {
			msg.Code = CoapCodeNotFound
		}

		if !IsExecutableResource(resource) {
			msg.Code = CoapCodeMethodNotAllowed
		} else {
			lwReq := Default(req, OPERATIONTYPE_EXECUTE)
			response := enabler.OnExecute(instanceId, resourceId, lwReq)
			msg.Code = response.GetResponseCode()
		}
	} else {
		msg.Code = CoapCodeNotFound
	}
	return NewResponseWithMessage(msg)
}

// Events
func (c *DefaultClient) OnStartup(fn FnOnStartup) {
	c.evtOnStartup = fn
}

func (c *DefaultClient) OnRead(fn FnOnRead) {
	c.evtOnRead = fn
}

func (c *DefaultClient) OnWrite(fn FnOnWrite) {
	c.evtOnWrite = fn
}

func (c *DefaultClient) OnExecute(fn FnOnExecute) {
	c.evtOnExecute = fn
}

func (c *DefaultClient) OnRegistered(fn FnOnRegistered) {
	c.evtOnRegistered = fn
}

func (c *DefaultClient) OnDeregistered(fn FnOnDeregistered) {
	c.evtOnDeregistered = fn
}

func (c *DefaultClient) OnError(fn FnOnError) {
	c.evtOnError = fn
}

func (c *DefaultClient) OnObserve(fn FnOnError) {

}
