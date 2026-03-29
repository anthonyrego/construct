// building_lib.scad — Parametric building modules for NYC architectural detail.
// Coordinate convention: XY = floor plan, Z = height (Z-up).
// The asset pipeline's STL converter swaps Y↔Z to match the engine's Y-up.
// All dimensions in meters.

// --- Core ---

// Extrude a 2D footprint polygon to a given height.
module building_shell(footprint, height) {
    linear_extrude(height=height)
        polygon(footprint);
}

// --- Windows ---

// Single recessed window opening (subtract from a wall).
// Place at the window's bottom-left corner on the wall surface.
module window(w, h, depth=0.12) {
    translate([0, 0, 0])
        cube([w, depth * 2, h]);
}

// Grid of recessed windows on a rectangular wall section.
// Anchor: bottom-left of the wall area, facing +Y direction.
// margin: space from wall edges to first/last window.
module window_grid(wall_w, wall_h, rows, cols, win_w=1.0, win_h=1.5, depth=0.12, margin=0.8) {
    if (cols > 0 && rows > 0) {
        usable_w = wall_w - 2 * margin;
        usable_h = wall_h - 2 * margin;
        spacing_x = (cols > 1) ? (usable_w - win_w) / (cols - 1) : 0;
        spacing_z = (rows > 1) ? (usable_h - win_h) / (rows - 1) : 0;

        for (c = [0:cols-1]) {
            for (r = [0:rows-1]) {
                x = margin + c * spacing_x;
                z = margin + r * spacing_z;
                translate([x, -depth, z])
                    cube([win_w, depth * 2, win_h]);
            }
        }
    }
}

// Apply window recesses to a wall by subtracting from the building shell.
// face_origin: [x, y] position of the wall's bottom-left corner.
// face_angle: rotation in degrees around Z axis to align with the wall normal.
module wall_windows(face_origin, face_angle, wall_w, wall_h, rows, cols,
                    win_w=1.0, win_h=1.5, depth=0.12, margin=0.8) {
    translate([face_origin[0], face_origin[1], 0])
        rotate([0, 0, face_angle])
            window_grid(wall_w, wall_h, rows, cols, win_w, win_h, depth, margin);
}

// --- Stoop (Front Entrance) ---

// NYC-style stoop with stairs.
// Anchor: center-bottom of the stoop at ground level, stairs extend in +Y.
module stoop(width=3.0, depth=2.0, steps=5, step_h=0.18) {
    total_h = steps * step_h;
    step_d = depth / steps;

    for (s = [0:steps-1]) {
        translate([-(width/2), s * step_d, s * step_h])
            cube([width, step_d + 0.01, total_h - s * step_h]);
    }

    // Landing platform at the top
    translate([-(width/2), depth, total_h - step_h])
        cube([width, 0.8, step_h]);
}

// --- Cornice ---

// Decorative cornice along the top edge of a building.
// Wraps around the footprint at the given height.
module cornice(footprint, height, depth=0.3, cornice_h=0.4) {
    n = len(footprint);
    for (i = [0:n-1]) {
        j = (i + 1) % n;
        p1 = footprint[i];
        p2 = footprint[j];

        dx = p2[0] - p1[0];
        dy = p2[1] - p1[1];
        edge_len = sqrt(dx*dx + dy*dy);
        angle = atan2(dy, dx);

        // Outward normal direction (right side of edge for CCW polygon)
        nx = dy / edge_len;
        ny = -dx / edge_len;

        translate([p1[0] + nx * depth/2, p1[1] + ny * depth/2, height])
            rotate([0, 0, angle])
                cube([edge_len, depth, cornice_h]);
    }
}

// --- Parapet ---

// Low wall around the roof edge.
module parapet(footprint, height, thickness=0.2, parapet_h=0.8) {
    difference() {
        // Outer volume
        linear_extrude(height=height + parapet_h)
            offset(delta=thickness)
                polygon(footprint);
        // Inner cutout (everything below parapet top)
        translate([0, 0, -0.01])
            linear_extrude(height=height + 0.02)
                polygon(footprint);
        // Inner cutout above roof level
        translate([0, 0, height])
            linear_extrude(height=parapet_h + 0.02)
                offset(delta=-0.01)
                    polygon(footprint);
    }
}

// --- Fire Escape ---

// Simple fire escape structure on a wall face.
// Anchor: bottom of the fire escape at the wall surface.
module fire_escape(width=2.5, floors=4, floor_h=3.0, depth=1.2, start_floor=1) {
    rail_w = 0.05;
    platform_h = 0.05;

    for (f = [start_floor:start_floor+floors-1]) {
        z = f * floor_h - 0.3;

        // Platform
        translate([0, 0, z])
            cube([width, depth, platform_h]);

        // Rails
        translate([0, 0, z])
            cube([rail_w, depth, 1.0]);
        translate([width - rail_w, 0, z])
            cube([rail_w, depth, 1.0]);

        // Front rail
        translate([0, depth - rail_w, z])
            cube([width, rail_w, 1.0]);
    }

    // Ladder between platforms
    ladder_w = 0.4;
    for (f = [start_floor:start_floor+floors-2]) {
        z = f * floor_h - 0.3;
        translate([width/2 - ladder_w/2, depth * 0.7, z + platform_h])
            cube([ladder_w, 0.05, floor_h]);
    }
}

// --- Storefront ---

// Ground-floor commercial storefront opening.
// Subtract this from the building shell.
module storefront(width=4.0, height=3.5, depth=0.3, door_w=1.2) {
    // Large display window
    win_w = (width - door_w) / 2 - 0.3;

    // Left window
    translate([0.15, -depth, 0.3])
        cube([win_w, depth * 2, height - 0.6]);

    // Door opening
    translate([win_w + 0.3, -depth, 0])
        cube([door_w, depth * 2, height - 0.3]);

    // Right window
    translate([win_w + door_w + 0.45, -depth, 0.3])
        cube([win_w, depth * 2, height - 0.6]);
}

// --- Setback ---

// Upper floor setback (common in taller NYC buildings).
// Creates a narrower volume above a given height.
module setback(footprint, total_height, setback_height, inset=2.0) {
    // Lower portion at full width
    linear_extrude(height=setback_height)
        polygon(footprint);

    // Upper portion inset
    translate([0, 0, setback_height])
        linear_extrude(height=total_height - setback_height)
            offset(delta=-inset)
                polygon(footprint);
}

// --- Utility ---

// Compute the length of a polygon edge.
function edge_length(footprint, i) =
    let(j = (i + 1) % len(footprint),
        dx = footprint[j][0] - footprint[i][0],
        dy = footprint[j][1] - footprint[i][1])
    sqrt(dx*dx + dy*dy);

// Compute the angle of a polygon edge (degrees).
function edge_angle(footprint, i) =
    let(j = (i + 1) % len(footprint),
        dx = footprint[j][0] - footprint[i][0],
        dy = footprint[j][1] - footprint[i][1])
    atan2(dy, dx);

// Compute the midpoint of a polygon edge.
function edge_midpoint(footprint, i) =
    let(j = (i + 1) % len(footprint))
    [(footprint[i][0] + footprint[j][0]) / 2,
     (footprint[i][1] + footprint[j][1]) / 2];

// Find the index of the longest edge in a polygon.
function longest_edge(footprint, i=0, best_i=0, best_len=0) =
    (i >= len(footprint)) ? best_i :
    let(el = edge_length(footprint, i))
    longest_edge(footprint, i+1,
                 (el > best_len) ? i : best_i,
                 (el > best_len) ? el : best_len);
