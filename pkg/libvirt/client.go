// pkg/libvirt/client.go
package libvirt

import (
    "encoding/xml"
    "fmt"
    "strings"
    
    "libvirt.org/go/libvirt"
)

// Client Libvirt客户端封装
type Client struct {
    conn *libvirt.Connect
    uri  string
}

// NewClient 创建新的libvirt客户端
func NewClient(uri string) (*Client, error) {
    conn, err := libvirt.NewConnect("qemu:///system")
    if err != nil {
        return nil, fmt.Errorf("failed to connect to libvirt: %w", err)
    }
    
    return &Client{
        conn: conn,
        uri:  uri,
    }, nil
}

// Close 关闭连接
func (c *Client) Close() error {
    if c.conn != nil {
        ret, err := c.conn.Close()
        if err != nil {
            return err
        }
        if ret != 0 {
            return fmt.Errorf("libvirt connection close returned code %d", ret)
        }
    }
    return nil
}

// DomainConfig 虚拟机配置
type DomainConfig struct {
    Name     string
    Memory   int
    VCPU     int
    DiskPath string
    Networks []NetworkConfig
}

type NetworkConfig struct {
    NetworkName string
    MAC         string
}

// Domain XML结构
type Domain struct {
    XMLName xml.Name `xml:"domain"`
    Type    string   `xml:"type,attr"`
    Name    string   `xml:"name"`
    Memory  Memory   `xml:"memory"`
    VCPU    VCPU     `xml:"vcpu"`
    OS      OS       `xml:"os"`
    Devices Devices  `xml:"devices"`
}

type Memory struct {
    Unit  string `xml:"unit,attr"`
    Value int    `xml:",chardata"`
}

type VCPU struct {
    Placement string `xml:"placement,attr"`
    Value     int    `xml:",chardata"`
}

type OS struct {
    Type OSType `xml:"type"`
    Boot Boot   `xml:"boot"`
}

type OSType struct {
    Arch    string `xml:"arch,attr"`
    Machine string `xml:"machine,attr"`
    Value   string `xml:",chardata"`
}

type Boot struct {
    Dev string `xml:"dev,attr"`
}

type Devices struct {
    Disks      []Disk      `xml:"disk"`
    Interfaces []Interface `xml:"interface"`
    Graphics   Graphics    `xml:"graphics"`
    Console    Console     `xml:"console"`
}

type Disk struct {
    Type   string     `xml:"type,attr"`
    Device string     `xml:"device,attr"`
    Driver DiskDriver `xml:"driver"`
    Source DiskSource `xml:"source"`
    Target DiskTarget `xml:"target"`
}

type DiskDriver struct {
    Name string `xml:"name,attr"`
    Type string `xml:"type,attr"`
}

type DiskSource struct {
    File string `xml:"file,attr"`
}

type DiskTarget struct {
    Dev string `xml:"dev,attr"`
    Bus string `xml:"bus,attr"`
}

type Interface struct {
    Type   string        `xml:"type,attr"`
    MAC    InterfaceMAC  `xml:"mac"`
    Source InterfaceSource `xml:"source"`
    Model  InterfaceModel  `xml:"model"`
}

type InterfaceMAC struct {
    Address string `xml:"address,attr"`
}

type InterfaceSource struct {
    Network string `xml:"network,attr"`
}

type InterfaceModel struct {
    Type string `xml:"type,attr"`
}

type Graphics struct {
    Type string `xml:"type,attr"`
    Port string `xml:"port,attr"`
}

type Console struct {
    Type   string        `xml:"type,attr"`
    Target ConsoleTarget `xml:"target"`
}

type ConsoleTarget struct {
    Type string `xml:"type,attr"`
    Port string `xml:"port,attr"`
}

// CreateDomain 创建虚拟机
func (c *Client) CreateDomain(config DomainConfig) (*libvirt.Domain, error) {
    // 构建虚拟机XML配置
    domain := Domain{
        Type: "kvm",
        Name: config.Name,
        Memory: Memory{
            Unit:  "KiB",
            Value: config.Memory * 1024,
        },
        VCPU: VCPU{
            Placement: "static",
            Value:     config.VCPU,
        },
        OS: OS{
            Type: OSType{
                Arch:    "x86_64",
                Machine: "pc-i440fx-2.1",
                Value:   "hvm",
            },
            Boot: Boot{Dev: "hd"},
        },
        Devices: Devices{
            Disks: []Disk{
                {
                    Type:   "file",
                    Device: "disk",
                    Driver: DiskDriver{
                        Name: "qemu",
                        Type: "qcow2",
                    },
                    Source: DiskSource{File: config.DiskPath},
                    Target: DiskTarget{
                        Dev: "vda",
                        Bus: "virtio",
                    },
                },
            },
            Graphics: Graphics{
                Type: "vnc",
                Port: "-1",
            },
            Console: Console{
                Type: "pty",
                Target: ConsoleTarget{
                    Type: "serial",
                    Port: "0",
                },
            },
        },
    }
    
    // 添加网络接口
    for _, net := range config.Networks {
        iface := Interface{
            Type: "network",
            MAC:  InterfaceMAC{Address: net.MAC},
            Source: InterfaceSource{Network: net.NetworkName},
            Model:  InterfaceModel{Type: "virtio"},
        }
        domain.Devices.Interfaces = append(domain.Devices.Interfaces, iface)
    }
    
    // 序列化为XML
    xmlData, err := xml.MarshalIndent(domain, "", "  ")
    if err != nil {
        return nil, fmt.Errorf("failed to marshal domain XML: %w", err)
    }
    
    // 创建虚拟机
    dom, err := c.conn.DomainDefineXML(string(xmlData))
    if err != nil {
        return nil, fmt.Errorf("failed to define domain: %w", err)
    }
    
    return dom, nil
}

// GetDomain 获取虚拟机
func (c *Client) GetDomain(name string) (*libvirt.Domain, error) {
    return c.conn.LookupDomainByName(name)
}

// DeleteDomain 删除虚拟机
func (c *Client) DeleteDomain(name string) error {
    dom, err := c.GetDomain(name)
    if err != nil {
        if strings.Contains(err.Error(), "not found") {
            return nil // 已经不存在
        }
        return err
    }
    defer dom.Free()
    
    // 停止虚拟机
    if err := dom.Destroy(); err != nil {
        // 忽略已经停止的错误
        if !strings.Contains(err.Error(), "not running") {
            return fmt.Errorf("failed to destroy domain: %w", err)
        }
    }
    
    // 删除定义
    return dom.Undefine()
}

// StartDomain 启动虚拟机
func (c *Client) StartDomain(name string) error {
    dom, err := c.GetDomain(name)
    if err != nil {
        return err
    }
    defer dom.Free()
    
    return dom.Create()
}

// StopDomain 停止虚拟机
func (c *Client) StopDomain(name string) error {
    dom, err := c.GetDomain(name)
    if err != nil {
        return err
    }
    defer dom.Free()
    
    return dom.Shutdown()
}

// GetDomainState 获取虚拟机状态
func (c *Client) GetDomainState(name string) (libvirt.DomainState, error) {
    dom, err := c.GetDomain(name)
    if err != nil {
        return libvirt.DOMAIN_NOSTATE, err
    }
    defer dom.Free()

    state, _, err := dom.GetState()
    return state, err
}

// GetDomainIPs 获取虚拟机IP地址
func (c *Client) GetDomainIPs(name string) ([]string, error) {
    dom, err := c.GetDomain(name)
    if err != nil {
        return nil, err
    }
    defer dom.Free()
    
    ifaces, err := dom.ListAllInterfaceAddresses(libvirt.DOMAIN_INTERFACE_ADDRESSES_SRC_LEASE)
    if err != nil {
        return nil, err
    }
    
    var ips []string
    for _, iface := range ifaces {
        for _, addr := range iface.Addrs {
            if addr.Type == libvirt.IP_ADDR_TYPE_IPV4 {
                ips = append(ips, addr.Addr)
            }
        }
    }
    
    return ips, nil
}

// EnsureNetwork 确保网络存在
func (c *Client) EnsureNetwork(name, cidr string) error {
    // 检查网络是否存在
    _, err := c.conn.LookupNetworkByName(name)
    if err == nil {
        return nil // 网络已存在
    }
    
    // 创建网络XML
    networkXML := fmt.Sprintf(`
<network>
  <name>%s</name>
  <forward mode='nat'/>
  <bridge name='virbr-%s' stp='on' delay='0'/>
  <ip address='%s' netmask='255.255.255.0'>
    <dhcp>
      <range start='%s' end='%s'/>
    </dhcp>
  </ip>
</network>`, name, name, getNetworkIP(cidr), getDHCPStart(cidr), getDHCPEnd(cidr))
    
    // 定义网络
    net, err := c.conn.NetworkDefineXML(networkXML)
    if err != nil {
        return fmt.Errorf("failed to define network: %w", err)
    }
    defer net.Free()
    
    // 启动网络
    if err := net.Create(); err != nil {
        return fmt.Errorf("failed to start network: %w", err)
    }
    
    // 设置自动启动
    return net.SetAutostart(true)
}

func getNetworkIP(cidr string) string {
    // 简单实现，实际应该解析CIDR
    return "192.168.122.1"
}

func getDHCPStart(cidr string) string {
    return "192.168.122.2"
}

func getDHCPEnd(cidr string) string {
    return "192.168.122.254"
}