package moe.matsuri.nb4a.proxy.tmsplit

import android.app.Application
import android.graphics.drawable.AnimationDrawable
import android.view.ContextThemeWrapper
import android.view.View
import android.widget.ImageView
import android.widget.TextView
import io.nekohasekai.sagernet.R
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [28], application = Application::class)
class TmSplitProgressViewTest {
    @Test fun firefoxFramesAndConnectionCaption() {
        val context = ContextThemeWrapper(RuntimeEnvironment.getApplication(), R.style.Theme_SagerNet)
        val view = TmSplitProgressView(context)
        assertEquals(2, view.childCount)
        val image = view.getChildAt(0) as ImageView
        val frames = image.drawable as AnimationDrawable
        assertEquals(30, frames.numberOfFrames)
        assertFalse(frames.isOneShot)
        repeat(30) { assertEquals(35, frames.getDuration(it)) }
        assertEquals("Установка соединения", (view.getChildAt(1) as TextView).text.toString())
        view.visibility = View.GONE
        assertFalse(frames.isRunning)
    }
}
