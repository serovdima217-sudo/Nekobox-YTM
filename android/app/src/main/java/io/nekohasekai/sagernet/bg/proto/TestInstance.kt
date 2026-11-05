package io.nekohasekai.sagernet.bg.proto

import io.nekohasekai.sagernet.BuildConfig
import io.nekohasekai.sagernet.bg.GuardedProcessPool
import io.nekohasekai.sagernet.database.ProxyEntity
import io.nekohasekai.sagernet.fmt.buildConfig
import io.nekohasekai.sagernet.ktx.Logs
import io.nekohasekai.sagernet.ktx.runOnDefaultDispatcher
import kotlinx.coroutines.delay
import libcore.Libcore
import moe.matsuri.nb4a.net.LocalResolverImpl
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.Job
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException
import java.util.concurrent.atomic.AtomicBoolean

class TestInstance(profile: ProxyEntity, val link: String, private val timeout: Int) :
    BoxInstance(profile) {

    suspend fun doTest(): Int {
        return suspendCancellableCoroutine { c ->
            var worker: Job? = null
            val completed = AtomicBoolean(false)
            fun failed(error: Exception) {
                if (completed.compareAndSet(false, true)) c.resumeWithException(error)
            }
            processes = GuardedProcessPool {
                Logs.w(it)
                failed(it)
                worker?.cancel()
            }
            worker = runOnDefaultDispatcher {
                use {
                    try {
                        init()
                        launch()
                        awaitTmSplitReady()
                        if (processes.processCount > 0 && profile.configBean?.type != 2) {
                            // wait for plugin start
                            delay(500)
                        }
                        val result = Libcore.urlTest(box, link, timeout)
                        if (completed.compareAndSet(false, true)) c.resume(result)
                    } catch (e: Exception) {
                        failed(e)
                    }
                }
            }
            c.invokeOnCancellation { completed.set(true); worker?.cancel() }
        }
    }

    override fun buildConfig() {
        config = buildConfig(profile, true)
    }

    override suspend fun loadConfig() {
        // don't call destroyAllJsi here
        if (BuildConfig.DEBUG) Logs.d(config.config)
        box = Libcore.newSingBoxInstance(config.config, LocalResolverImpl)
    }

}
