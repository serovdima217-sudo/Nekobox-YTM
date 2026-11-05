package moe.matsuri.nb4a.proxy.tmsplit

import kotlinx.coroutines.TimeoutCancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.delay
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeout
import java.io.IOException
import java.net.InetSocketAddress
import java.net.Socket

// Таймер URL-запроса начинается только после настоящей готовности SOCKS.
// Ожидание отменяемое; фиксированная задержка в 20 секунд не используется.
suspend fun awaitTmSplitSocks(ports: Collection<Int>, timeoutMs: Long = 240_000L) {
    if (ports.isEmpty()) return
    try { withTimeout(timeoutMs) {
        withContext(Dispatchers.IO) {
            val pending = ports.toMutableSet()
            while (pending.isNotEmpty()) {
                currentCoroutineContext().ensureActive()
                val iterator = pending.iterator()
                while (iterator.hasNext()) {
                    currentCoroutineContext().ensureActive()
                    val port = iterator.next()
                    val ready = try {
                        Socket().use { socket ->
                            socket.soTimeout = 500
                            socket.connect(InetSocketAddress("127.0.0.1", port), 250)
                            socket.getOutputStream().apply { write(byteArrayOf(5, 1, 0)); flush() }
                            val input = socket.getInputStream()
                            input.read() == 5 && input.read() == 0
                        }
                    } catch (_: IOException) { false }
                    if (ready) iterator.remove()
                }
                if (pending.isNotEmpty()) delay(250)
            }
        }
    } } catch (_: TimeoutCancellationException) {
        throw IOException("Не удалось установить соединение SplitYTM: свободная пара не подготовилась за отведённое время")
    }
}
