use </Users/rego/projects/construct/data/scad/building_lib.scad>;

footprint = [
  [-14.9362, -62.8200],
  [-8.5549, -58.6358],
  [-19.6163, -32.6755],
  [-30.1259, -39.1142]
];

height = 28.96;
floors = 9;
floor_h = 3.22;

// Building shell with window recesses
difference() {
  building_shell(footprint, height);

  // Window recesses
  wall_windows([-14.9362, -62.8200], 33.25, 7.63, height, 9, 3, 1.00, 1.50);
  wall_windows([-8.5549, -58.6358], 113.08, 28.22, height, 9, 10, 1.00, 1.50);
  wall_windows([-19.6163, -32.6755], -148.51, 12.33, height, 9, 4, 1.00, 1.50);
  wall_windows([-30.1259, -39.1142], -57.35, 28.15, height, 9, 10, 1.00, 1.50);
}

// Cornice
cornice(footprint, height, 0.25, 0.35);

// Stoop
translate([-13.6256, -45.4597, 0])
  rotate([0, 0, 203.08])
    stoop(2.5, 1.8, 4, 0.18);

