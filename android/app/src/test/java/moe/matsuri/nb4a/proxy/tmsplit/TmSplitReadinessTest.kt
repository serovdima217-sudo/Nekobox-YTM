package moe.matsuri.nb4a.proxy.tmsplit

import kotlinx.coroutines.*
import org.junit.Assert.*
import org.junit.Test
import java.io.IOException
import java.net.ServerSocket
import kotlin.concurrent.thread

class TmSplitReadinessTest {
    @Test fun emptyPortsReturnImmediately() = runBlocking {
        awaitTmSplitSocks(emptyList(), 100)
    }
    @Test fun waitsForRealSocksHandshake() = runBlocking {
        val server = ServerSocket(0)
        val worker = thread {
            server.use {
                it.accept().use { socket ->
                    val input = socket.getInputStream()
                    assertEquals(5, input.read()); assertEquals(1, input.read()); assertEquals(0, input.read())
                    socket.getOutputStream().apply { write(byteArrayOf(5, 0)); flush() }
                }
            }
        }
        awaitTmSplitSocks(listOf(server.localPort), 2000)
        worker.join(2000)
        assertFalse(worker.isAlive)
    }
    @Test fun openTcpPortWithoutSocksIsNotReady() = runBlocking {
        val server = ServerSocket(0)
        val worker = thread(isDaemon = true) {
            try {
                while (!server.isClosed) server.accept().use { socket -> socket.getOutputStream().write(byteArrayOf(1, 1)) }
            } catch (_: IOException) { }
        }
        try {
            try { awaitTmSplitSocks(listOf(server.localPort), 150); fail("Неверный протокол принят за SOCKS") }
            catch (_: IOException) { }
        } finally { server.close(); worker.join(1000) }
    }
    @Test fun cancellationDoesNotWaitForFullDeadline() = runBlocking {
        val port = ServerSocket(0).use { it.localPort }
        val task = launch { awaitTmSplitSocks(listOf(port), 60_000) }
        delay(50)
        withTimeout(2000) { task.cancelAndJoin() }
        assertTrue(task.isCancelled)
    }
    @Test fun waitsPastOldHalfSecondLimit() = runBlocking {
        val server = ServerSocket(0)
        val worker = thread {
            server.use {
                it.accept().use { socket ->
                    val input = socket.getInputStream()
                    repeat(3) { input.read() }
                    Thread.sleep(650)
                    runCatching { socket.getOutputStream().write(byteArrayOf(5, 0)) }
                }
                it.accept().use { socket ->
                    val input = socket.getInputStream(); repeat(3) { input.read() }
                    socket.getOutputStream().apply { write(byteArrayOf(5, 0)); flush() }
                }
            }
        }
        try { awaitTmSplitSocks(listOf(server.localPort), 3000) }
        finally { server.close(); worker.join(2000) }
    }
}
