package moe.matsuri.nb4a.proxy.tmsplit

import android.content.Context
import android.graphics.drawable.AnimationDrawable
import android.view.Gravity
import android.view.View
import android.widget.LinearLayout
import android.widget.TextView
import androidx.appcompat.widget.AppCompatImageView
import io.nekohasekai.sagernet.R

// Воспроизводятся исходные кадры Firefox, а не похожая анимация окружностей.
class TmSplitProgressView(context: Context) : LinearLayout(context) {
    private lateinit var icon: AppCompatImageView
    private fun dp(value: Int) = (value * resources.displayMetrics.density).toInt()
    init {
        orientation = VERTICAL
        gravity = Gravity.CENTER
        minimumWidth = dp(260)
        minimumHeight = dp(176)
        setPadding(dp(24), dp(24), dp(24), dp(24))
        icon = AppCompatImageView(context).apply {
            setImageResource(R.drawable.tm_firefox_loading)
            importantForAccessibility = View.IMPORTANT_FOR_ACCESSIBILITY_NO
        }
        addView(icon, LayoutParams(dp(64), dp(64)))
        addView(TextView(context).apply {
            setText(R.string.tm_split_connecting)
            textSize = 16f
            gravity = Gravity.CENTER
            accessibilityLiveRegion = View.ACCESSIBILITY_LIVE_REGION_POLITE
        }, LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.WRAP_CONTENT).apply { topMargin = dp(12) })
    }
    private fun updateAnimation() {
        if (!::icon.isInitialized) return
        (icon.drawable as? AnimationDrawable)?.let {
            if (isShown && isAttachedToWindow) it.start() else it.stop()
        }
    }
    override fun onAttachedToWindow() { super.onAttachedToWindow(); updateAnimation() }
    override fun onDetachedFromWindow() {
        (icon.drawable as? AnimationDrawable)?.stop()
        super.onDetachedFromWindow()
    }
    override fun onVisibilityChanged(changedView: View, visibility: Int) {
        super.onVisibilityChanged(changedView, visibility)
        updateAnimation()
    }
}
