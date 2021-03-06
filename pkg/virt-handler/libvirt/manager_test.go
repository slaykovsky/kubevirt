package libvirt

import (
	"encoding/xml"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/rgbkrk/libvirt-go"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/tools/record"
	"kubevirt.io/kubevirt/pkg/api/v1"
)

var _ = Describe("Manager", func() {
	var mockConn *MockConnection
	var mockDomain *MockVirDomain
	var ctrl *gomock.Controller
	var recorder *record.FakeRecorder

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockConn = NewMockConnection(ctrl)
		mockDomain = NewMockVirDomain(ctrl)
		recorder = record.NewFakeRecorder(10)
	})

	Context("on successful VM sync", func() {
		It("should define and start a new VM", func() {
			vm := newVM("testvm")
			mockConn.EXPECT().LookupDomainByName("testvm").Return(mockDomain, libvirt.VirError{Code: libvirt.VIR_ERR_NO_DOMAIN})
			xml, err := xml.Marshal(vm.Spec.Domain)
			Expect(err).To(BeNil())
			mockConn.EXPECT().DomainDefineXML(string(xml)).Return(mockDomain, nil)
			mockDomain.EXPECT().GetState().Return([]int{libvirt.VIR_DOMAIN_SHUTDOWN, -1}, nil)
			mockDomain.EXPECT().Create().Return(nil)
			manager, _ := NewLibvirtDomainManager(mockConn, recorder)
			err = manager.SyncVM(vm)
			Expect(err).To(BeNil())
			Expect(<-recorder.Events).To(ContainSubstring(v1.Created.String()))
			Expect(<-recorder.Events).To(ContainSubstring(v1.Started.String()))
			Expect(recorder.Events).To(BeEmpty())
		})
		It("should leave a defined and started VM alone", func() {
			mockConn.EXPECT().LookupDomainByName("testvm").Return(mockDomain, nil)
			mockDomain.EXPECT().GetState().Return([]int{libvirt.VIR_DOMAIN_RUNNING, -1}, nil)
			manager, _ := NewLibvirtDomainManager(mockConn, recorder)
			err := manager.SyncVM(newVM("testvm"))
			Expect(err).To(BeNil())
			Expect(recorder.Events).To(BeEmpty())
		})
		table.DescribeTable("should try to start a VM in state",
			func(state int) {
				mockConn.EXPECT().LookupDomainByName("testvm").Return(mockDomain, nil)
				mockDomain.EXPECT().GetState().Return([]int{state, -1}, nil)
				mockDomain.EXPECT().Create().Return(nil)
				manager, _ := NewLibvirtDomainManager(mockConn, recorder)
				err := manager.SyncVM(newVM("testvm"))
				Expect(err).To(BeNil())
				Expect(<-recorder.Events).To(ContainSubstring(v1.Started.String()))
				Expect(recorder.Events).To(BeEmpty())
			},
			table.Entry("crashed", libvirt.VIR_DOMAIN_CRASHED),
			table.Entry("shutdown", libvirt.VIR_DOMAIN_SHUTDOWN),
			table.Entry("shutoff", libvirt.VIR_DOMAIN_SHUTOFF),
			table.Entry("unknown", libvirt.VIR_DOMAIN_NOSTATE),
		)
		It("should resume a paused VM", func() {
			mockConn.EXPECT().LookupDomainByName("testvm").Return(mockDomain, nil)
			mockDomain.EXPECT().GetState().Return([]int{libvirt.VIR_DOMAIN_PAUSED, -1}, nil)
			mockDomain.EXPECT().Resume().Return(nil)
			manager, _ := NewLibvirtDomainManager(mockConn, recorder)
			err := manager.SyncVM(newVM("testvm"))
			Expect(err).To(BeNil())
			Expect(<-recorder.Events).To(ContainSubstring(v1.Resumed.String()))
			Expect(recorder.Events).To(BeEmpty())
		})
	})

	Context("on successful VM kill", func() {
		table.DescribeTable("should try to undefine a VM in state",
			func(state int) {
				mockConn.EXPECT().LookupDomainByName("testvm").Return(mockDomain, nil)
				mockDomain.EXPECT().GetState().Return([]int{state, -1}, nil)
				mockDomain.EXPECT().Undefine().Return(nil)
				manager, _ := NewLibvirtDomainManager(mockConn, recorder)
				err := manager.KillVM(newVM("testvm"))
				Expect(err).To(BeNil())
			},
			table.Entry("crashed", libvirt.VIR_DOMAIN_CRASHED),
			table.Entry("shutdown", libvirt.VIR_DOMAIN_SHUTDOWN),
			table.Entry("shutoff", libvirt.VIR_DOMAIN_SHUTOFF),
			table.Entry("unknown", libvirt.VIR_DOMAIN_NOSTATE),
		)
		table.DescribeTable("should try to destroy and undefine a VM in state",
			func(state int) {
				mockConn.EXPECT().LookupDomainByName("testvm").Return(mockDomain, nil)
				mockDomain.EXPECT().GetState().Return([]int{state, -1}, nil)
				mockDomain.EXPECT().Destroy().Return(nil)
				mockDomain.EXPECT().Undefine().Return(nil)
				manager, _ := NewLibvirtDomainManager(mockConn, recorder)
				err := manager.KillVM(newVM("testvm"))
				Expect(err).To(BeNil())
				Expect(<-recorder.Events).To(ContainSubstring(v1.Stopped.String()))
				Expect(<-recorder.Events).To(ContainSubstring(v1.Deleted.String()))
				Expect(recorder.Events).To(BeEmpty())
			},
			table.Entry("running", libvirt.VIR_DOMAIN_RUNNING),
			table.Entry("paused", libvirt.VIR_DOMAIN_PAUSED),
		)
	})

	// TODO: test error reporting on non successful VM syncs and kill attempts

	AfterEach(func() {
		ctrl.Finish()
	})
})

func newVM(name string) *v1.VM {
	return &v1.VM{
		ObjectMeta: api.ObjectMeta{Name: name},
		Spec:       v1.VMSpec{Domain: v1.NewMinimalVM(name)},
	}
}
