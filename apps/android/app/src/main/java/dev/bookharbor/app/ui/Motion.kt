package dev.bookharbor.app.ui

import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.spring
import androidx.compose.foundation.interaction.InteractionSource
import androidx.compose.foundation.interaction.collectIsPressedAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.composed
import androidx.compose.ui.graphics.graphicsLayer

/** Sinks slightly while pressed and springs back, so a tap on a cover or card feels physical. */
fun Modifier.pressScale(interactions: InteractionSource, pressed: Float = 0.96f): Modifier = composed {
    val isPressed by interactions.collectIsPressedAsState()
    val scale by animateFloatAsState(if (isPressed) pressed else 1f, spring(dampingRatio = Spring.DampingRatioMediumBouncy, stiffness = Spring.StiffnessMediumLow), label = "press")
    graphicsLayer { scaleX = scale; scaleY = scale }
}
