package io.nekohasekai.sagernet.linux
import io.nekohasekai.sagernet.fmt.AbstractBean
import moe.matsuri.nb4a.proxy.anytls.AnyTLSBean
import moe.matsuri.nb4a.proxy.shadowtls.ShadowTLSBean
import io.nekohasekai.sagernet.database.DataStore
import io.nekohasekai.sagernet.fmt.http.HttpBean
import io.nekohasekai.sagernet.fmt.hysteria.HysteriaBean
import io.nekohasekai.sagernet.fmt.mieru.MieruBean
import io.nekohasekai.sagernet.fmt.naive.NaiveBean
import io.nekohasekai.sagernet.fmt.shadowsocks.ShadowsocksBean
import io.nekohasekai.sagernet.fmt.socks.SOCKSBean
import io.nekohasekai.sagernet.fmt.ssh.SSHBean
import io.nekohasekai.sagernet.fmt.trojan.TrojanBean
import io.nekohasekai.sagernet.fmt.trojan_go.TrojanGoBean
import io.nekohasekai.sagernet.fmt.tuic.TuicBean
import io.nekohasekai.sagernet.fmt.v2ray.StandardV2RayBean
import io.nekohasekai.sagernet.fmt.v2ray.VMessBean
import io.nekohasekai.sagernet.fmt.wireguard.WireGuardBean
import io.nekohasekai.sagernet.ktx.applyDefaultValues
import moe.matsuri.nb4a.proxy.PreferenceBinding
import moe.matsuri.nb4a.proxy.PreferenceBindingManager
import moe.matsuri.nb4a.proxy.Type
interface LinuxEditor { fun load(bean:AbstractBean);fun save(bean:AbstractBean) }
class HysteriaLinuxEditor : LinuxEditor {

 override fun load(bean: AbstractBean) { with(bean as HysteriaBean) {
        DataStore.profileName = name
        DataStore.protocolVersion = protocolVersion
        DataStore.serverAddress = serverAddress
        DataStore.serverPorts = serverPorts
        DataStore.serverObfs = obfuscation
        DataStore.serverAuthType = authPayloadType
        DataStore.serverProtocolInt = protocol
        DataStore.serverPassword = authPayload
        DataStore.serverSNI = sni
        DataStore.serverALPN = alpn
        DataStore.serverCertificates = caText
        DataStore.serverAllowInsecure = allowInsecure
        DataStore.serverUploadSpeed = uploadMbps
        DataStore.serverDownloadSpeed = downloadMbps
        DataStore.serverStreamReceiveWindow = streamReceiveWindow
        DataStore.serverConnectionReceiveWindow = connectionReceiveWindow
        DataStore.serverDisableMtuDiscovery = disableMtuDiscovery
        DataStore.serverHopInterval = hopInterval
    
} }
 override fun save(bean: AbstractBean) { with(bean as HysteriaBean) {
        name = DataStore.profileName
        protocolVersion = DataStore.protocolVersion
        serverAddress = DataStore.serverAddress
        serverPorts = DataStore.serverPorts
        obfuscation = DataStore.serverObfs
        authPayloadType = DataStore.serverAuthType
        authPayload = DataStore.serverPassword
        protocol = DataStore.serverProtocolInt
        sni = DataStore.serverSNI
        alpn = DataStore.serverALPN
        caText = DataStore.serverCertificates
        allowInsecure = DataStore.serverAllowInsecure
        uploadMbps = DataStore.serverUploadSpeed
        downloadMbps = DataStore.serverDownloadSpeed
        streamReceiveWindow = DataStore.serverStreamReceiveWindow
        connectionReceiveWindow = DataStore.serverConnectionReceiveWindow
        disableMtuDiscovery = DataStore.serverDisableMtuDiscovery
        hopInterval = DataStore.serverHopInterval
    
} }
}
class MieruLinuxEditor : LinuxEditor {

 override fun load(bean: AbstractBean) { with(bean as MieruBean) {
        DataStore.profileName = name
        DataStore.serverAddress = serverAddress
        DataStore.serverPort = serverPort
        DataStore.serverProtocol = protocol
        DataStore.serverUsername = username
        DataStore.serverPassword = password
        DataStore.serverMTU = mtu
    
} }
 override fun save(bean: AbstractBean) { with(bean as MieruBean) {
        name = DataStore.profileName
        serverAddress = DataStore.serverAddress
        serverPort = DataStore.serverPort
        protocol = DataStore.serverProtocol
        username = DataStore.serverUsername
        password = DataStore.serverPassword
        mtu = DataStore.serverMTU
    
} }
}
class NaiveLinuxEditor : LinuxEditor {

 override fun load(bean: AbstractBean) { with(bean as NaiveBean) {
        DataStore.profileName = name
        DataStore.serverAddress = serverAddress
        DataStore.serverPort = serverPort
        DataStore.serverUsername = username
        DataStore.serverPassword = password
        DataStore.serverProtocol = proto
        DataStore.serverSNI = sni
        DataStore.serverCertificates = certificates
        DataStore.serverHeaders = extraHeaders
        DataStore.serverInsecureConcurrency = insecureConcurrency
        DataStore.profileCacheStore.putBoolean("sUoT", sUoT)
    
} }
 override fun save(bean: AbstractBean) { with(bean as NaiveBean) {
        name = DataStore.profileName
        serverAddress = DataStore.serverAddress
        serverPort = DataStore.serverPort
        username = DataStore.serverUsername
        password = DataStore.serverPassword
        proto = DataStore.serverProtocol
        sni = DataStore.serverSNI
        certificates = DataStore.serverCertificates
        extraHeaders = DataStore.serverHeaders.replace("\r\n", "\n")
        insecureConcurrency = DataStore.serverInsecureConcurrency
        sUoT = DataStore.profileCacheStore.getBoolean("sUoT")
    
} }
}
class SSHLinuxEditor : LinuxEditor {

 override fun load(bean: AbstractBean) { with(bean as SSHBean) {
        DataStore.profileName = name
        DataStore.serverAddress = serverAddress
        DataStore.serverPort = serverPort
        DataStore.serverUsername = username
        DataStore.serverAuthType = authType
        DataStore.serverPassword = password
        DataStore.serverPrivateKey = privateKey
        DataStore.serverPassword1 = privateKeyPassphrase
        DataStore.serverCertificates = publicKey
    
} }
 override fun save(bean: AbstractBean) { with(bean as SSHBean) {
        name = DataStore.profileName
        serverAddress = DataStore.serverAddress
        serverPort = DataStore.serverPort
        username = DataStore.serverUsername
        authType = DataStore.serverAuthType
        when (authType) {
            SSHBean.AUTH_TYPE_NONE -> {
            }
            SSHBean.AUTH_TYPE_PASSWORD -> {
                password = DataStore.serverPassword
            }
            SSHBean.AUTH_TYPE_PRIVATE_KEY -> {
                privateKey = DataStore.serverPrivateKey
                privateKeyPassphrase = DataStore.serverPassword1
            }
        }
        publicKey = DataStore.serverCertificates
    
} }
}
class ShadowsocksLinuxEditor : LinuxEditor {
private val pbm = PreferenceBindingManager()
private val name = pbm.add(PreferenceBinding(Type.Text, "name"))
private val serverAddress = pbm.add(PreferenceBinding(Type.Text, "serverAddress"))
private val serverPort = pbm.add(PreferenceBinding(Type.TextToInt, "serverPort"))
private val password = pbm.add(PreferenceBinding(Type.Text, "password"))
private val method = pbm.add(PreferenceBinding(Type.Text, "method"))
private val pluginName =
        pbm.add(PreferenceBinding(Type.Text, "pluginName").apply { disable = true })
private val pluginConfig =
        pbm.add(PreferenceBinding(Type.Text, "pluginConfig").apply { disable = true })
private val sUoT = pbm.add(PreferenceBinding(Type.Bool, "sUoT"))
 override fun load(bean: AbstractBean) { with(bean as ShadowsocksBean) {
        pbm.writeToCacheAll(this)

        DataStore.profileCacheStore.putString("pluginName", plugin.substringBefore(";"))
        DataStore.profileCacheStore.putString("pluginConfig", plugin.substringAfter(";"))
    
} }
 override fun save(bean: AbstractBean) { with(bean as ShadowsocksBean) {
        pbm.fromCacheAll(this)

        val pn = pluginName.readStringFromCache()
        val pc = pluginConfig.readStringFromCache()
        plugin = if (pn.isNotBlank()) "$pn;$pc" else ""
    
} }
}
class SocksLinuxEditor : LinuxEditor {

 override fun load(bean: AbstractBean) { with(bean as SOCKSBean) {
        DataStore.profileName = name
        DataStore.serverAddress = serverAddress
        DataStore.serverPort = serverPort

        DataStore.serverProtocolInt = protocol
        DataStore.serverUsername = username
        DataStore.serverPassword = password

        DataStore.profileCacheStore.putBoolean("sUoT", sUoT)
    
} }
 override fun save(bean: AbstractBean) { with(bean as SOCKSBean) {
        name = DataStore.profileName
        serverAddress = DataStore.serverAddress
        serverPort = DataStore.serverPort

        protocol = DataStore.serverProtocolInt
        username = DataStore.serverUsername
        password = DataStore.serverPassword

        sUoT = DataStore.profileCacheStore.getBoolean("sUoT")
    
} }
}
class StandardV2RayLinuxEditor : LinuxEditor {
private val pbm = PreferenceBindingManager()
private val name = pbm.add(PreferenceBinding(Type.Text, "name"))
private val serverAddress = pbm.add(PreferenceBinding(Type.Text, "serverAddress"))
private val serverPort = pbm.add(PreferenceBinding(Type.TextToInt, "serverPort"))
private val uuid = pbm.add(PreferenceBinding(Type.Text, "uuid"))
private val username = pbm.add(PreferenceBinding(Type.Text, "username"))
private val password = pbm.add(PreferenceBinding(Type.Text, "password"))
private val alterId = pbm.add(PreferenceBinding(Type.TextToInt, "alterId"))
private val encryption = pbm.add(PreferenceBinding(Type.Text, "encryption"))
private val type = pbm.add(PreferenceBinding(Type.Text, "type"))
private val host = pbm.add(PreferenceBinding(Type.Text, "host"))
private val path = pbm.add(PreferenceBinding(Type.Text, "path"))
private val packetEncoding = pbm.add(PreferenceBinding(Type.TextToInt, "packetEncoding"))
private val wsMaxEarlyData = pbm.add(PreferenceBinding(Type.TextToInt, "wsMaxEarlyData"))
private val earlyDataHeaderName = pbm.add(PreferenceBinding(Type.Text, "earlyDataHeaderName"))
private val security = pbm.add(PreferenceBinding(Type.Text, "security"))
private val sni = pbm.add(PreferenceBinding(Type.Text, "sni"))
private val alpn = pbm.add(PreferenceBinding(Type.Text, "alpn"))
private val certificates = pbm.add(PreferenceBinding(Type.Text, "certificates"))
private val allowInsecure = pbm.add(PreferenceBinding(Type.Bool, "allowInsecure"))
private val utlsFingerprint = pbm.add(PreferenceBinding(Type.Text, "utlsFingerprint"))
private val realityPubKey = pbm.add(PreferenceBinding(Type.Text, "realityPubKey"))
private val realityShortId = pbm.add(PreferenceBinding(Type.Text, "realityShortId"))
private val enableECH = pbm.add(PreferenceBinding(Type.Bool, "enableECH"))
private val echConfig = pbm.add(PreferenceBinding(Type.Text, "echConfig"))
private val enableMux = pbm.add(PreferenceBinding(Type.Bool, "enableMux"))
private val muxPadding = pbm.add(PreferenceBinding(Type.Bool, "muxPadding"))
private val muxType = pbm.add(PreferenceBinding(Type.TextToInt, "muxType"))
private val muxConcurrency = pbm.add(PreferenceBinding(Type.TextToInt, "muxConcurrency"))
 override fun load(bean: AbstractBean) { with(bean as StandardV2RayBean) {
        if (this is TrojanBean) {
            this@StandardV2RayLinuxEditor.uuid.fieldName = "password"
            this@StandardV2RayLinuxEditor.password.disable = true
        }

        
        pbm.writeToCacheAll(this)
    
} }
 override fun save(bean: AbstractBean) { with(bean as StandardV2RayBean) {
        pbm.fromCacheAll(this)
    
} }
}
class TrojanGoLinuxEditor : LinuxEditor {

 override fun load(bean: AbstractBean) { with(bean as TrojanGoBean) {
        DataStore.profileName = name
        DataStore.serverAddress = serverAddress
        DataStore.serverPort = serverPort
        DataStore.serverPassword = password
        DataStore.serverSNI = sni
        DataStore.serverAllowInsecure = allowInsecure
        DataStore.serverNetwork = type
        DataStore.serverHost = host
        DataStore.serverPath = path
        if (encryption.startsWith("ss;")) {
            DataStore.serverEncryption = "ss"
            DataStore.serverMethod = encryption.substringAfter(";").substringBefore(":")
            DataStore.serverPassword1 = encryption.substringAfter(":")
        } else {
            DataStore.serverEncryption = encryption
        }
    
} }
 override fun save(bean: AbstractBean) { with(bean as TrojanGoBean) {
        name = DataStore.profileName
        serverAddress = DataStore.serverAddress
        serverPort = DataStore.serverPort
        password = DataStore.serverPassword
        sni = DataStore.serverSNI
        allowInsecure = DataStore.serverAllowInsecure
        type = DataStore.serverNetwork
        host = DataStore.serverHost
        path = DataStore.serverPath
        encryption = when (val security = DataStore.serverEncryption) {
            "ss" -> {
                "ss;" + DataStore.serverMethod + ":" + DataStore.serverPassword1
            }
            else -> {
                security
            }
        }
    
} }
}
class TuicLinuxEditor : LinuxEditor {

 override fun load(bean: AbstractBean) { with(bean as TuicBean) {
        DataStore.profileName = name
        DataStore.serverAddress = serverAddress
        DataStore.serverPort = serverPort
        DataStore.serverUsername = uuid
        DataStore.serverPassword = token
        DataStore.serverALPN = alpn
        DataStore.serverCertificates = caText
        DataStore.serverUDPRelayMode = udpRelayMode
        DataStore.serverCongestionController = congestionController
        DataStore.serverDisableSNI = disableSNI
        DataStore.serverSNI = sni
        DataStore.serverReduceRTT = reduceRTT
        DataStore.serverAllowInsecure = allowInsecure
    
} }
 override fun save(bean: AbstractBean) { with(bean as TuicBean) {
        name = DataStore.profileName
        serverAddress = DataStore.serverAddress
        serverPort = DataStore.serverPort
        uuid = DataStore.serverUsername
        token = DataStore.serverPassword
        alpn = DataStore.serverALPN
        caText = DataStore.serverCertificates
        udpRelayMode = DataStore.serverUDPRelayMode
        congestionController = DataStore.serverCongestionController
        disableSNI = DataStore.serverDisableSNI
        sni = DataStore.serverSNI
        reduceRTT = DataStore.serverReduceRTT
        allowInsecure = DataStore.serverAllowInsecure
    
} }
}
class WireGuardLinuxEditor : LinuxEditor {
private val pbm = PreferenceBindingManager()
private val name = pbm.add(PreferenceBinding(Type.Text, "name"))
private val serverAddress = pbm.add(PreferenceBinding(Type.Text, "serverAddress"))
private val serverPort = pbm.add(PreferenceBinding(Type.TextToInt, "serverPort"))
private val localAddress = pbm.add(PreferenceBinding(Type.Text, "localAddress"))
private val privateKey = pbm.add(PreferenceBinding(Type.Text, "privateKey"))
private val peerPublicKey = pbm.add(PreferenceBinding(Type.Text, "peerPublicKey"))
private val peerPreSharedKey = pbm.add(PreferenceBinding(Type.Text, "peerPreSharedKey"))
private val mtu = pbm.add(PreferenceBinding(Type.TextToInt, "mtu"))
private val reserved = pbm.add(PreferenceBinding(Type.Text, "reserved"))
 override fun load(bean: AbstractBean) { with(bean as WireGuardBean) {
        pbm.writeToCacheAll(this)
    
} }
 override fun save(bean: AbstractBean) { with(bean as WireGuardBean) {
        pbm.fromCacheAll(this)
    
} }
}
class AnyTLSLinuxEditor : LinuxEditor {
private val pbm = PreferenceBindingManager()
private val name = pbm.add(PreferenceBinding(Type.Text, "name"))
private val serverAddress = pbm.add(PreferenceBinding(Type.Text, "serverAddress"))
private val serverPort = pbm.add(PreferenceBinding(Type.TextToInt, "serverPort"))
private val password = pbm.add(PreferenceBinding(Type.Text, "password"))
private val sni = pbm.add(PreferenceBinding(Type.Text, "sni"))
private val alpn = pbm.add(PreferenceBinding(Type.Text, "alpn"))
private val certificates = pbm.add(PreferenceBinding(Type.Text, "certificates"))
private val allowInsecure = pbm.add(PreferenceBinding(Type.Bool, "allowInsecure"))
private val utlsFingerprint = pbm.add(PreferenceBinding(Type.Text, "utlsFingerprint"))
 override fun load(bean: AbstractBean) { with(bean as AnyTLSBean) {
        pbm.writeToCacheAll(this)

    
} }
 override fun save(bean: AbstractBean) { with(bean as AnyTLSBean) {
        pbm.fromCacheAll(this)
    
} }
}
class ShadowTLSLinuxEditor : LinuxEditor {
private val pbm = PreferenceBindingManager()
private val name = pbm.add(PreferenceBinding(Type.Text, "name"))
private val serverAddress = pbm.add(PreferenceBinding(Type.Text, "serverAddress"))
private val serverPort = pbm.add(PreferenceBinding(Type.TextToInt, "serverPort"))
private val password = pbm.add(PreferenceBinding(Type.Text, "password"))
private val version = pbm.add(PreferenceBinding(Type.TextToInt, "version"))
private val sni = pbm.add(PreferenceBinding(Type.Text, "sni"))
private val alpn = pbm.add(PreferenceBinding(Type.Text, "alpn"))
private val certificates = pbm.add(PreferenceBinding(Type.Text, "certificates"))
private val allowInsecure = pbm.add(PreferenceBinding(Type.Bool, "allowInsecure"))
private val utlsFingerprint = pbm.add(PreferenceBinding(Type.Text, "utlsFingerprint"))
 override fun load(bean: AbstractBean) { with(bean as ShadowTLSBean) {
        pbm.writeToCacheAll(this)

    
} }
 override fun save(bean: AbstractBean) { with(bean as ShadowTLSBean) {
        pbm.fromCacheAll(this)
    
} }
}
fun editorFor(bean:AbstractBean):LinuxEditor? = when(bean) {
 is io.nekohasekai.sagernet.fmt.internal.ChainBean -> ChainLinuxEditor()
 is HysteriaBean -> HysteriaLinuxEditor()
 is MieruBean -> MieruLinuxEditor()
 is NaiveBean -> NaiveLinuxEditor()
 is SSHBean -> SSHLinuxEditor()
 is ShadowsocksBean -> ShadowsocksLinuxEditor()
 is SOCKSBean -> SocksLinuxEditor()
 is TrojanGoBean -> TrojanGoLinuxEditor()
 is TuicBean -> TuicLinuxEditor()
 is WireGuardBean -> WireGuardLinuxEditor()
 is AnyTLSBean -> AnyTLSLinuxEditor()
 is ShadowTLSBean -> ShadowTLSLinuxEditor()
 is StandardV2RayBean -> StandardV2RayLinuxEditor()
 else->null
}
