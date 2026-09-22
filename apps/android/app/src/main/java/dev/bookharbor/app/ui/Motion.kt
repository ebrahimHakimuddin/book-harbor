package dev.bookharbor.app.ui

import androidx.compose.animation.core.FiniteAnimationSpec
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.spring
import androidx.compose.foundation.interaction.InteractionSource
import androidx.compose.foundation.interaction.collectIsPressedAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.composed
import androidx.compose.ui.graphics.graphicsLayer

/**
 * The app's motion vocabulary. UI moves on critically damped springs -- they start from wherever
 * the value is now, so any animation can be interrupted and reversed without a jump -- and only
 * settles without overshoot. Bounce is reserved for moments the user physically "throws".
 */
object Motion {
    /** Default for anything that moves: no overshoot, settles in about 0.35 s. */
    fun <T> standard(): FiniteAnimationSpec<T> = spring(dampingRatio = Spring.DampingRatioNoBouncy, stiffness = 400f)
    /** Press feedback: fast enough to feel instant under the finger. */
    fun <T> press(): FiniteAnimationSpec<T> = spring(dampingRatio = Spring.DampingRatioNoBouncy, stiffness = 1200f)
}

/** Sinks slightly the moment a finger lands (not on release) and settles back without bounce. */
fun Modifier.pressScale(interactions: InteractionSource, pressed: Float = 0.96f): Modifier = composed {
    val isPressed by interactions.collectIsPressedAsState()
    val scale by animateFloatAsState(if (isPressed) pressed else 1f, Motion.press(), label = "press")
    graphicsLayer { scaleX = scale; scaleY = scale }
}
